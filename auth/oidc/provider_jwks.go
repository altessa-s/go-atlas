// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sync/atomic"
	"time"

	"github.com/MicahParks/jwkset"
	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"

	"github.com/altessa-s/go-atlas-core/runtime/cycle"
	"github.com/altessa-s/go-atlas-core/runtime/singleflight"

	corectx "github.com/altessa-s/go-atlas-core/context"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	authjwt "github.com/altessa-s/go-atlas/auth/jwt"
)

type jwksKeySource struct {
	schedule              string
	info                  *discoveryInfo
	client                *http.Client
	logger                *slog.Logger
	metrics               *oidcMetrics
	backgroundCtx         context.Context
	jwks                  keyfunc.Keyfunc
	keyResolver           authjwt.KeyResolver
	lastKeyFingerprint    atomic.Pointer[keyFingerprintMemo]
	jwksRefreshTask       *cycle.Guard
	refreshFlight         *singleflight.Group[struct{}, struct{}]
	lastUnknownKidAttempt *time.Time // owned by the shared refresh flight
	clock                 freshnessClock
	lastJWKSRefresh       atomic.Pointer[freshnessRecord]
	jwksHTTPTimeout       time.Duration
	jwksMaxStaleness      time.Duration
	jwksFailureMode       JWKSFailureMode
}

// jwksVerificationUses lists the JWK "use" values of a key that may verify a
// token: "sig", or no "use" at all. It is both the keyfunc whitelist and what
// [verificationEligible] goes by, so the two cannot drift.
var jwksVerificationUses = []jwkset.USE{jwkset.UseSig, ""}

// verificationEligible reports whether a key may verify a token, by the "use"
// member of its raw JWK — before the key is parsed, so a key that could never
// verify a token is never parsed either. Explicit key operations must permit verification.
func verificationEligible(marshal jwkset.JWKMarshal) bool {
	return slices.Contains(jwksVerificationUses, marshal.USE) &&
		(marshal.KEYOPS == nil || slices.Contains(marshal.KEYOPS, jwkset.KeyOpsVerify))
}

// parseJWKS converts the verification keys of a fetched JWK Set, for the
// initial load and for every refresh alike.
//
// A key that is not [verificationEligible] — an encryption key — is left out
// without being parsed: whatever is wrong with it, it cannot fail the set,
// and it could not verify a token anyway. Among the eligible keys, one of a
// type this build does not support ([jwkset.ErrUnsupportedKey]) is skipped,
// and any other invalid key fails the whole set: a document with a corrupted
// signing key is not a trustworthy picture of the IdP's keys, and applying
// the rest of it could drop keys still in use.
//
// Keys are unmarshaled with Private: true, so a symmetric (oct) key is
// accepted rather than failing the set; it never verifies a token by default
// because HS* is not in DefaultValidMethods.
func parseJWKS(marshals []jwkset.JWKMarshal) ([]jwkset.JWK, error) {
	keys := make([]jwkset.JWK, 0, len(marshals))
	for _, marshal := range marshals {
		if !verificationEligible(marshal) {
			continue
		}
		key, err := jwkset.NewJWKFromMarshal(marshal, jwkset.JWKMarshalOptions{Private: true}, jwkset.JWKValidateOptions{})
		if errors.Is(err, jwkset.ErrUnsupportedKey) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("invalid JWK: %w", err)
		}
		keys = append(keys, key)
	}
	return keys, nil
}

// initializeJWKS loads bounded JWKS storage; the provider owns subsequent refreshes.
func (p *jwksKeySource) initializeJWKS(ctx context.Context) error {
	// Initialization preserves the dependency's strict validation policy and
	// default timeout, while all network reads share the bounded fetch path.
	timeout := p.jwksHTTPTimeout
	if timeout == 0 {
		timeout = time.Minute
	}
	ctx, cancel := corectx.ApplyTimeout(ctx, timeout)
	defer cancel()
	jwks, err := p.fetchJWKS(ctx)
	if err != nil {
		return coreerrs.WrapOperation(err, "initialize JWKS storage")
	}
	keys, err := parseJWKS(jwks.Keys)
	if err != nil {
		return coreerrs.WrapOperation(err, "initialize JWK")
	}
	p.refreshFlight = singleflight.New[struct{}, struct{}](singleflight.WithTimeout(timeout))
	storage := jwkset.NewMemoryStorage()
	if err = storage.KeyReplaceAll(ctx, keys); err != nil {
		return coreerrs.WrapOperation(err, "initialize JWKS storage")
	}

	// Create keyfunc with the configured storage. Only signature keys verify a
	// token: a JWK whose "use" is "sig" or absent. Without the whitelist a
	// token without "kid" would be tried against every key in the set,
	// including encryption ("enc") keys.
	p.jwks, err = keyfunc.New(keyfunc.Options{
		Ctx:          p.backgroundCtx,
		Storage:      storage,
		UseWhitelist: jwksVerificationUses,
	})
	if err != nil {
		return coreerrs.WrapOperation(err, "create JWKS keyfunc")
	}

	// Bridge the keyfunc JWKS cache to an authjwt.KeyResolver. keyfunc/v3 reads
	// only the "kid" and "alg" header fields (not token.Method), so a minimal
	// synthetic token carrying the verified header is enough to look up the key.
	// token.Method is intentionally nil here: it is never read by keyfunc/v3, so
	// this depends on that library behavior. If a future keyfunc version reads
	// token.Method.Alg() this would nil-panic — the go.mod pin and the JWKS
	// tests guard against that. The resolved key's Algorithm is left empty so
	// authjwt's allow-list (WithAllowedAlgorithms) governs the accepted
	// algorithms; authjwt still rejects a nil key.
	p.keyResolver = authjwt.KeyResolverFunc(func(ctx context.Context, hdr authjwt.Header, _ authjwt.Claims) (authjwt.VerificationKey, error) {
		tok := &jwt.Token{Header: map[string]any{"alg": hdr.Alg}}
		if hdr.Kid != "" {
			tok.Header["kid"] = hdr.Kid
		}
		lookup := p.jwks.KeyfuncCtx(ctx)
		observed := p.lastJWKSRefresh.Load()
		key, err := lookup(tok)
		if hdr.Kid != "" && errors.Is(err, jwkset.ErrKeyNotFound) {
			if refreshErr := p.refreshJWKS(ctx, observed); refreshErr == nil || errors.Is(refreshErr, errUnknownKidRefreshSkipped) {
				key, err = lookup(tok)
			}
			if errors.Is(err, jwkset.ErrKeyNotFound) {
				return authjwt.VerificationKey{}, jwkset.ErrKeyNotFound
			}
		}
		if err != nil {
			return authjwt.VerificationKey{}, err
		}
		return authjwt.VerificationKey{Key: key}, nil
	})

	// Seed the staleness anchor so [jwksKeySource.checkJWKSStaleness] does not
	// reject every request issued before the first scheduled refresh.
	// Construction is the first authoritative "fresh-as-of-now" event.
	p.markJWKSRefreshed()

	return nil
}

// markJWKSRefreshed records the current monotonic instant as the last
// authoritative JWKS refresh. Callers MUST invoke it only when the
// storage has been fully repopulated from the IdP.
func (p *jwksKeySource) markJWKSRefreshed() {
	p.lastJWKSRefresh.Store(&freshnessRecord{observedAt: p.clock.observe()})
}

// checkJWKSStaleness reacts when the local JWKS cache has not been
// refreshed within [WithJWKSMaxStaleness]. Behavior is controlled by
// [WithJWKSFailureMode]:
//
//   - [JWKSFailureModeEnforce] (default) returns [ErrJWKSStale] so the
//     caller rejects the validation;
//   - [JWKSFailureModeWarn] logs at error level and lets the validation
//     proceed;
//   - [JWKSFailureModeDisabled] is a no-op.
//
// A zero [WithJWKSMaxStaleness] disables the check regardless of mode,
// preserving the historical opt-in behavior so callers without an
// active refresh path are not surprised by sudden rejections.
func (p *jwksKeySource) checkJWKSStaleness(ctx context.Context) error {
	maxStaleness := p.jwksMaxStaleness
	if maxStaleness <= 0 {
		return nil
	}
	mode := p.jwksFailureMode
	if mode == JWKSFailureModeDisabled {
		return nil
	}

	last := p.lastJWKSRefresh.Load()
	if last == nil {
		return nil
	}
	age := p.clock.elapsed(last.observedAt)
	if age <= maxStaleness {
		return nil
	}

	p.metrics.jwksStaleRejections.Inc()
	switch mode {
	case JWKSFailureModeWarn:
		p.logger.ErrorContext(ctx,
			"JWKS cache exceeded staleness budget but JWKSFailureModeWarn is configured; allowing validation",
			slog.Duration("age", age),
			slog.Duration("max_staleness", maxStaleness))
		return nil
	default: // JWKSFailureModeEnforce — also covers empty/unknown modes.
		p.logger.ErrorContext(ctx,
			"JWKS cache exceeded staleness budget; rejecting validation",
			slog.Duration("age", age),
			slog.Duration("max_staleness", maxStaleness))
		return coreerrs.Wrapf(ErrJWKSStale,
			"jwks age %s exceeds max staleness %s", age, maxStaleness)
	}
}

const defaultJWKSUnknownKidRefreshInterval = time.Minute

var errUnknownKidRefreshSkipped = errors.New("oidc: unknown-key JWKS refresh skipped")

// refreshJWKSInternal is the entry point used by scheduled and manual refreshes.
func (p *jwksKeySource) refreshJWKSInternal(ctx context.Context) error {
	for {
		err := p.refreshJWKS(ctx, nil)
		if !errors.Is(err, errUnknownKidRefreshSkipped) {
			return err
		}
		// A scheduled or manual refresh cannot be throttled, even if it
		// joined an unknown-key flight that was.
	}
}

// refreshJWKS coalesces fetches across scheduled, manual, and unknown-key
// refreshes. A non-nil observed record identifies a lookup-triggered refresh.
func (p *jwksKeySource) refreshJWKS(ctx context.Context, observed *freshnessRecord) error {
	_, err := p.refreshFlight.Do(ctx, struct{}{}, func(ctx context.Context) (struct{}, error) {
		if observed != nil {
			if p.lastJWKSRefresh.Load() != observed {
				return struct{}{}, errUnknownKidRefreshSkipped
			}
			if p.lastUnknownKidAttempt != nil && p.clock.elapsed(*p.lastUnknownKidAttempt) < defaultJWKSUnknownKidRefreshInterval {
				return struct{}{}, errUnknownKidRefreshSkipped
			}
			now := p.clock.observe()
			p.lastUnknownKidAttempt = &now
		}
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		stop := context.AfterFunc(p.backgroundCtx, cancel) //nolint:contextcheck // A shared refresh also inherits the provider lifetime.
		defer stop()
		return struct{}{}, p.fetchAndApplyJWKS(ctx)
	})
	return err
}

func (p *jwksKeySource) fetchAndApplyJWKS(ctx context.Context) error {
	// A closed provider does no work, whether RefreshJWKS is called directly
	// or by a cron tick that raced with Close.
	if err := p.backgroundCtx.Err(); err != nil {
		return coreerrs.Wrap(err, "oidc provider closed")
	}
	p.metrics.jwksRefreshes.Inc()
	stop := p.metrics.jwksRefreshDuration.Start()
	defer stop()

	refreshErr := p.doRefreshJWKS(ctx)
	if refreshErr != nil {
		p.metrics.jwksRefreshErrors.Inc()
		return refreshErr
	}
	p.markJWKSRefreshed()
	return nil
}

func (p *jwksKeySource) doRefreshJWKS(ctx context.Context) error {
	jwks, err := p.fetchJWKS(ctx)
	if err != nil {
		return err
	}

	// The same policy as the initial load: a malformed verification key fails
	// the refresh as a whole. Skipping it and applying the rest would evict
	// keys that are still signing tokens whenever a glitch corrupts some of
	// the entries. Keys that cannot verify a token are not parsed at all.
	newKeys, err := parseJWKS(jwks.Keys)
	if err != nil {
		return fmt.Errorf("%w: JWK Set holds an invalid key; keeping the current keys: %w", ErrDiscovery, err)
	}

	// A document without a key that can verify a token ({}, an empty set, only
	// unsupported keys, only "enc" keys of a half-published rotation) is a failed
	// refresh, not a rotation: replacing a working key set with it would
	// reject every token until the next successful refresh.
	if len(newKeys) == 0 {
		current, err := p.jwks.Storage().KeyReadAll(ctx)
		if err != nil {
			return coreerrs.WrapOperation(err, "read JWKS storage")
		}
		if len(current) > 0 {
			return coreerrs.Wrap(ErrDiscovery, "JWK Set holds no key usable for verification; keeping the current keys")
		}
	}

	if err := p.jwks.Storage().KeyReplaceAll(ctx, newKeys); err != nil {
		return coreerrs.WrapOperation(err, "update JWKS storage")
	}

	return nil
}
