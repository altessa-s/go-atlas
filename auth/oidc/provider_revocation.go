// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/altessa-s/go-atlas/cache/probfilter"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	coreretry "github.com/altessa-s/go-atlas-core/retry"
)

// checkIntrospection asks the IdP (RFC 7662) whether a signature-verified
// token is still active. It runs only after the token's signature and claims
// were verified, so garbage or foreign-signed tokens never reach the IdP.
//
// Introspection errors are fatal by default: the token is rejected with
// [ErrIntrospection] when the endpoint is unreachable ([WithIntrospection]).
// Enable [WithIntrospectionFailOpen] to instead log the error and accept the
// token on its signature alone.
func (p *Provider) checkIntrospection(ctx context.Context, token string) error {
	introspectionResp, err := p.IntrospectToken(ctx, token)
	if err != nil {
		p.metrics.revocationCheckErrors.Inc()
		if p.opts.introspectionFailOpen {
			p.logger.WarnContext(ctx, "token introspection failed, continuing with signature validation", slog.Any("error", err))
			return nil
		}
		p.logger.WarnContext(ctx,
			"token introspection failed, rejecting token",
			slog.Any("error", err))
		return coreerrs.Wrap(ErrIntrospection, "introspection endpoint unavailable")
	}
	if !introspectionResp.Active {
		return ErrTokenRevoked
	}
	return nil
}

// checkTokenRevocationVerified runs every revocation check for a token whose
// signature has been verified: the local revocation storage first (keyed on
// the full token, or on the verified jti claim / kid header), then RFC 7662
// introspection when enabled. A local hit always rejects — introspection
// cannot overrule it — and a local miss never skips introspection.
//
// Every validation path runs it after signature and claim verification, so
// all paths share the same revocation boundary. It must only be called with
// claims/header returned by a signature-verifying path
// ([Provider.verifyTokenSignature]); passing unverified data would let an
// attacker choose the jti/kid lookup key.
func (p *Provider) checkTokenRevocationVerified(ctx context.Context, token string, claims jwt.MapClaims, header map[string]any) error {
	if err := p.checkLocalRevocation(ctx, token, claims, header); err != nil {
		return err
	}
	if p.opts.introspectionEnabled {
		return p.checkIntrospection(ctx, token)
	}
	return nil
}

// checkLocalRevocation consults the configured [RevocationStorage]. A storage
// error rejects the token with [ErrRevocationCheck] unless
// [WithRevocationFailOpen] is set; either way it is logged and counted.
func (p *Provider) checkLocalRevocation(ctx context.Context, token string, claims jwt.MapClaims, header map[string]any) error {
	if p.revocationStorage == nil {
		return nil
	}

	item := revocationItemFromVerifiedClaims(p.opts.revocationItemType, claims, header, token)
	isRevoked, err := p.revocationStorage.IsRevoked(ctx, item)
	if err == nil && !isRevoked {
		err = p.checkRevocationStaleness()
	}
	if err != nil {
		p.metrics.revocationCheckErrors.Inc()
		if p.opts.revocationFailOpen {
			p.logger.ErrorContext(ctx, "revocation storage check failed; allowing token (fail-open)",
				"item_type", p.opts.revocationItemType, "error", err)
			return nil
		}
		p.logger.ErrorContext(ctx, "revocation storage check failed; rejecting token (fail-closed)",
			"item_type", p.opts.revocationItemType, "error", err)
		return fmt.Errorf("%w: revocation storage: %w", ErrRevocationCheck, err)
	}
	if isRevoked {
		p.logger.DebugContext(ctx, "item found in revocation storage",
			"item_type", p.opts.revocationItemType)
		return ErrTokenRevoked
	}

	return nil
}

// markRevocationSynced records now as the last successful revocation sync.
func (p *Provider) markRevocationSynced() {
	p.lastRevocationSyncUnixNanos.Store(time.Now().UnixNano())
}

// sharedRevocationSyncReadTimeout bounds the read of the shared sync time
// that follows a scheduled sync.
const sharedRevocationSyncReadTimeout = 5 * time.Second

// refreshSharedRevocationSync caches when the last sync committed by any
// replica happened, when the storage reports one ([RevocationSyncReporter]).
// The scheduled sync job calls it after every run, so validation never pays
// for the read.
//
// The storage's commit time belongs to another clock (the Redis server's), so
// it is never compared with the local one: the storage also reports the age
// of the commit on its own clock, and the cached local instant is now minus
// that age. A storage clock ahead of or behind the local clock therefore
// neither stretches nor shortens the bound. The instant is stored once per
// commit — a re-read of a commit already seen changes nothing, so reading
// can never refresh what no sync refreshed. A read that fails, or reports no
// commit, keeps the cached value. Only the sync job calls it, one run at a
// time.
func (p *Provider) refreshSharedRevocationSync(ctx context.Context) {
	reporter, ok := p.revocationStorage.(RevocationSyncReporter)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, sharedRevocationSyncReadTimeout)
	defer cancel()
	committed, age, known, err := reporter.LastCommittedSync(ctx)
	if err != nil {
		p.logger.WarnContext(ctx, "reading the shared revocation sync time failed", slog.Any("error", err))
		return
	}
	if !known || p.seenSharedRevocationCommit.Swap(committed.UnixNano()) == committed.UnixNano() {
		return
	}
	p.sharedRevocationSyncUnixNanos.Store(time.Now().Add(-max(age, 0)).UnixNano())
}

// checkRevocationStaleness returns [ErrRevocationStale] when no revocation
// sync was committed within [WithRevocationMaxStaleness]: a "not revoked"
// answer then comes from a snapshot too old to trust. The newest of this
// provider's own last successful sync and the last sync committed by any
// replica of a shared storage counts. A zero or negative bound disables the
// check, and so does the absence of a scheduled sync: nothing would ever
// refresh the snapshot.
func (p *Provider) checkRevocationStaleness() error {
	maxStaleness := p.opts.revocationMaxStaleness
	if maxStaleness <= 0 || !p.revocationSyncScheduled() {
		return nil
	}
	last := max(p.lastRevocationSyncUnixNanos.Load(), p.sharedRevocationSyncUnixNanos.Load())
	if last == 0 {
		return coreerrs.Wrap(ErrRevocationStale, "no successful revocation sync")
	}
	if age := time.Since(time.Unix(0, last)); age > maxStaleness {
		return coreerrs.Wrapf(ErrRevocationStale, "last sync %s ago exceeds max staleness %s", age, maxStaleness)
	}
	return nil
}

// Backoff bounds for retrying the initial revocation sync while a shared
// filter is busy (see initialRevocationSync).
const (
	initialSyncRetryBaseDelay = 250 * time.Millisecond
	initialSyncRetryMaxDelay  = 5 * time.Second
	initialSyncRetryJitter    = 0.2
)

// initialRevocationSync populates the revocation storage once at
// construction so the provider never serves with an empty filter while
// waiting for the first scheduled sync. Failure follows the revocation fail
// mode: fail-closed (default) aborts construction, fail-open logs and goes on.
func (p *Provider) initialRevocationSync(ctx context.Context) error {
	if p.revocationStorage == nil {
		return nil
	}
	// Only a sync this provider completed itself proves the filter holds the
	// current source. While another replica rebuilds the shared filter
	// (ErrRebuildInProgress) or this rebuild lost its lease
	// (ErrRebuildSuperseded) the sync is retried with backoff, bounded by
	// WithRevocationInitialSyncWait; a peer that publishes releases the lease
	// and the retry then rebuilds and publishes again, and a peer that fails
	// or vanishes is replaced by the retry. Any other error, or running out of
	// time, is a failed initial sync.
	// The budget bounds only the waiting between attempts: each attempt gets
	// the caller's context, so a long rebuild of our own is never cut off.
	waitCtx, cancel := context.WithTimeout(ctx, p.opts.revocationInitialSyncWait)
	defer cancel()
	var lastErr error
	err := coreretry.Do(waitCtx, func(context.Context) error {
		lastErr = p.revocationStorage.Sync(ctx)
		return lastErr
	},
		coreretry.WithMaxAttempts(-1),
		coreretry.WithShouldRetry(func(err error) bool {
			return errors.Is(err, probfilter.ErrRebuildInProgress) || errors.Is(err, probfilter.ErrRebuildSuperseded)
		}),
		coreretry.WithNextDelay(coreretry.Exponential(
			initialSyncRetryBaseDelay,
			coreretry.WithExponentialMaxDelay(initialSyncRetryMaxDelay),
			coreretry.WithExponentialJitter(initialSyncRetryJitter),
		)),
	)
	if err == nil {
		p.markRevocationSynced()
		return nil
	}
	if lastErr != nil && !errors.Is(err, lastErr) {
		err = errors.Join(lastErr, err) // budget exhausted while the filter stayed busy
	}
	p.metrics.revocationCheckErrors.Inc()
	if p.opts.revocationFailOpen {
		p.logger.ErrorContext(ctx, "initial revocation sync failed; starting anyway (fail-open)", "error", err)
		// Start the staleness clock at construction, so the scheduled sync has
		// the whole bound to catch up before checks count as stale.
		p.markRevocationSynced()
		return nil
	}
	return fmt.Errorf("%w: initial revocation sync: %w", ErrRevocationCheck, err)
}

// revocationItemFromVerifiedClaims extracts the revocation lookup key from
// signature-verified claims/header. Falls back to the full token when the
// configured field is missing so the caller still gets a deterministic key.
func revocationItemFromVerifiedClaims(itemType string, claims jwt.MapClaims, header map[string]any, token string) string {
	switch itemType {
	case RevocationItemTypeJTI:
		if jti, ok := claims[RevocationItemTypeJTI].(string); ok && jti != "" {
			return jti
		}
	case RevocationItemTypeKID:
		if header != nil {
			if kid, ok := header[RevocationItemTypeKID].(string); ok && kid != "" {
				return kid
			}
		}
	}
	return token
}
