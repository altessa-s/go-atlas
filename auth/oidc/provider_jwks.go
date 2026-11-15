// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/MicahParks/jwkset"
	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"

	corectx "github.com/altessa-s/go-atlas-core/context"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	authjwt "github.com/altessa-s/go-atlas/auth/jwt"
)

// jwksVerificationUses lists the JWK "use" values of a key that may verify a
// token: "sig", or no "use" at all. It is both the keyfunc whitelist and what
// [verificationEligible] goes by, so the two cannot drift.
var jwksVerificationUses = []jwkset.USE{jwkset.UseSig, ""}

// verificationEligible reports whether a key may verify a token, by the "use"
// member of its raw JWK — before the key is parsed, so a key that could never
// verify a token is never parsed either. "key_ops" is not consulted: the
// keyfunc that resolves verification keys selects by "use" alone. Algorithms
// are not a property of the key set here: the validation options' allow-list
// decides them per token.
func verificationEligible(marshal jwkset.JWKMarshal) bool {
	return slices.Contains(jwksVerificationUses, marshal.USE)
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
			return nil, fmt.Errorf("JWK %q: %w", marshal.KID, err)
		}
		keys = append(keys, key)
	}
	return keys, nil
}

// initializeJWKS loads bounded JWKS storage; the provider owns subsequent refreshes.
func (p *Provider) initializeJWKS(ctx context.Context) error {
	// Initialization preserves the dependency's strict validation policy and
	// default timeout, while all network reads share the bounded fetch path.
	timeout := p.opts.jwksHTTPTimeout
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
		key, err := p.jwks.KeyfuncCtx(ctx)(tok)
		if err != nil {
			return authjwt.VerificationKey{}, err
		}
		return authjwt.VerificationKey{Key: key}, nil
	})

	// Seed the staleness anchor so [Provider.checkJWKSStaleness] does not
	// reject every request issued before the first scheduled refresh.
	// Construction is the first authoritative "fresh-as-of-now" event.
	p.markJWKSRefreshed()

	return nil
}

// markJWKSRefreshed records the current wall-clock time as the last
// authoritative JWKS refresh. Callers MUST invoke it only when the
// storage has been fully repopulated from the IdP (initial bootstrap,
// successful scheduled refresh, successful manual [Provider.RefreshJWKS]).
func (p *Provider) markJWKSRefreshed() {
	p.lastJWKSRefreshUnixNanos.Store(time.Now().UnixNano())
}

// jwksAge returns the time elapsed since the last successful JWKS refresh.
// Returns zero if no successful refresh has been recorded yet (the field
// is seeded in [Provider.initializeJWKS], so in practice this only happens
// in unit tests that bypass NewProvider).
func (p *Provider) jwksAge() time.Duration {
	stored := p.lastJWKSRefreshUnixNanos.Load()
	if stored == 0 {
		return 0
	}
	age := time.Since(time.Unix(0, stored))
	if age < 0 {
		return 0
	}
	return age
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
func (p *Provider) checkJWKSStaleness(ctx context.Context) error {
	maxStaleness := p.opts.jwksMaxStaleness
	if maxStaleness <= 0 {
		return nil
	}
	mode := p.opts.jwksFailureMode
	if mode == JWKSFailureModeDisabled {
		return nil
	}

	age := p.jwksAge()
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
