// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/altessa-s/go-atlas-core/types/redacted"
	"github.com/altessa-s/go-atlas/cache/probfilter"
	"github.com/altessa-s/go-atlas/internal/diagnostics"

	corecontext "github.com/altessa-s/go-atlas-core/context"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	coreretry "github.com/altessa-s/go-atlas-core/retry"
)

type revocationChecker struct {
	info          *discoveryInfo
	client        *http.Client
	logger        *slog.Logger
	metrics       *oidcMetrics
	backgroundCtx context.Context
	tokenCache    Cacher

	itemType                    string
	skipMissingItem             bool
	failOpen                    bool
	initialSyncWait             time.Duration
	maxStaleness                time.Duration
	syncTimeout                 time.Duration
	syncSchedule                string
	introspectionEnabled        bool
	introspectionClientID       string
	introspectionSecret         redacted.RedactedString
	introspectionFailOpen       bool
	revokedTokensCacheKeyPrefix string
	activeTokensCacheKeyPrefix  string
	filterCleanup               func(context.Context) error
	storage                     RevocationStorage
	loader                      probfilter.DataLoader
	syncMu                      sync.Mutex
	clock                       freshnessClock
	lastRevocationSync          atomic.Pointer[freshnessRecord]
	sharedRevocationSync        atomic.Pointer[freshnessRecord]
	// Only the serialized sync job compares peer commit identities.
	seenSharedRevocationCommit time.Time
}

func newRevocationChecker(o *options, lifetime context.Context, metrics *oidcMetrics) revocationChecker {
	var schedule string
	if o.revocationSyncEnabled {
		schedule = o.revocationSyncSchedule
	}
	return revocationChecker{
		syncSchedule:                schedule,
		logger:                      o.logger,
		metrics:                     metrics,
		backgroundCtx:               lifetime,
		tokenCache:                  o.tokenCache,
		storage:                     o.revocationStorage,
		loader:                      o.revocationLoader,
		itemType:                    o.revocationItemType,
		skipMissingItem:             o.revocationSkipMissingItem,
		failOpen:                    o.revocationFailOpen,
		initialSyncWait:             o.revocationInitialSyncWait,
		maxStaleness:                o.revocationMaxStaleness,
		syncTimeout:                 o.revocationSyncTimeout,
		introspectionEnabled:        o.introspectionEnabled,
		introspectionClientID:       o.introspectionClientID,
		introspectionSecret:         o.introspectionSecret,
		introspectionFailOpen:       o.introspectionFailOpen,
		revokedTokensCacheKeyPrefix: o.revokedTokensCacheKeyPrefix,
		activeTokensCacheKeyPrefix:  o.activeTokensCacheKeyPrefix,
		filterCleanup:               o.revocationFilterCleanup,
	}
}

// checkIntrospection asks the IdP (RFC 7662) whether a signature-verified
// token is still active. It runs only after the token's signature and claims
// were verified, so garbage or foreign-signed tokens never reach the IdP.
//
// Introspection errors are fatal by default: the token is rejected with
// [ErrIntrospection] when the endpoint is unreachable ([WithIntrospection]).
// Enable [WithIntrospectionFailOpen] to instead log the error and accept the
// token on its signature alone.
func (p *revocationChecker) checkIntrospection(ctx context.Context, token string) error {
	introspectionResp, err := p.introspectToken(ctx, token)
	if err != nil {
		p.metrics.revocationCheckErrors.Inc()
		if p.introspectionFailOpen {
			p.logger.WarnContext(ctx, "token introspection failed, continuing with signature validation", slog.Any("error", err))
			return nil
		}
		p.logger.WarnContext(ctx,
			"token introspection failed, rejecting token",
			slog.Any("error", err))
		return err
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
// ([policyEvaluator.verifyTokenSignature]); passing unverified data would let an
// attacker choose the jti/kid lookup key.
func (p *revocationChecker) checkTokenRevocationVerified(ctx context.Context, token string, claims jwt.MapClaims, header map[string]any) error {
	if err := p.checkLocalRevocation(ctx, token, claims, header); err != nil {
		return err
	}
	if p.introspectionEnabled {
		return p.checkIntrospection(ctx, token)
	}
	return nil
}

// checkLocalRevocation consults the configured [RevocationStorage]. A storage
// error rejects the token with [ErrRevocationCheck] unless
// [WithRevocationFailOpen] is set; either way it is logged and counted.
func (p *revocationChecker) checkLocalRevocation(ctx context.Context, token string, claims jwt.MapClaims, header map[string]any) error {
	if p.storage == nil {
		return nil
	}

	item, ok := revocationItemFromVerifiedClaims(p.itemType, claims, header, token)
	if !ok {
		if !p.skipMissingItem {
			p.metrics.revocationCheckErrors.Inc()
			p.logger.WarnContext(ctx, "token lacks the revocation item claim; rejecting token", "item_type", p.itemType)
			return coreerrs.Wrapf(ErrTokenInvalid, "token has no %s to check revocation", p.itemType)
		}
		item = token
	}
	isRevoked, err := p.storage.IsRevoked(ctx, item)
	if errors.Is(err, ErrRevocationReadinessLost) {
		p.startRevocationRecovery()
	}
	if err == nil && !isRevoked {
		err = p.checkRevocationStaleness()
	}
	if err != nil {
		p.metrics.revocationCheckErrors.Inc()
		err = boundaryError("check revocation", 0, ErrRevocationCheck, err)
		if p.failOpen {
			p.logger.ErrorContext(ctx, "revocation storage check failed; allowing token (fail-open)",
				"item_type", p.itemType, "error", err)
			return nil
		}
		p.logger.ErrorContext(ctx, "revocation storage check failed; rejecting token (fail-closed)",
			"item_type", p.itemType, "error", err)
		return err
	}
	if isRevoked {
		p.logger.DebugContext(ctx, "item found in revocation storage",
			"item_type", p.itemType)
		return ErrTokenRevoked
	}

	return nil
}

// markRevocationSynced records now as the last successful revocation sync.
func (p *revocationChecker) markRevocationSynced() {
	p.lastRevocationSync.Store(&freshnessRecord{observedAt: p.clock.observe()})
}

// sharedRevocationSyncReadTimeout bounds the read of the shared sync time
// that follows a scheduled sync.
const sharedRevocationSyncReadTimeout = 5 * time.Second

// refreshSharedRevocationSync caches when the last sync committed by any
// replica happened, when the storage reports one ([RevocationSyncReporter]).
// The scheduled sync job calls it after every run, so validation never pays
// for the read.
//
// The peer clock identifies the commit and supplies its age. Anchor that
// age before the RPC so transport latency always consumes the local budget.
func (p *revocationChecker) refreshSharedRevocationSync(ctx context.Context) {
	reporter, ok := p.storage.(RevocationSyncReporter)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, sharedRevocationSyncReadTimeout)
	defer cancel()
	observedAt := p.clock.observe()
	committed, age, known, err := reporter.LastCommittedSync(ctx)
	if err != nil {
		p.logger.WarnContext(ctx, "reading the shared revocation sync time failed", slog.String("error", diagnostics.ErrorClass(err)))
		return
	}
	if !known || committed.Equal(p.seenSharedRevocationCommit) {
		return
	}
	// Remember rejected commits too: a later clock setback at the peer must
	// not make the very same publication eligible for fresh credit.
	p.seenSharedRevocationCommit = committed
	budget := p.maxStaleness
	if age < 0 || age == math.MaxInt64 || (budget > 0 && age >= budget) {
		p.sharedRevocationSync.Store(nil)
		return
	}
	p.sharedRevocationSync.Store(&freshnessRecord{observedAt: observedAt, baseAge: age})
}

// checkRevocationStaleness returns [ErrRevocationStale] when no revocation
// sync was committed within [WithRevocationMaxStaleness]: a "not revoked"
// answer then comes from a snapshot too old to trust. The newest of this
// provider's own last successful sync and the last sync committed by any
// replica of a shared storage counts. A zero or negative bound disables the
// check, and so does the absence of a scheduled sync: nothing would ever
// refresh the snapshot.
func (p *revocationChecker) checkRevocationStaleness() error {
	maxStaleness := p.maxStaleness
	if maxStaleness <= 0 || !p.syncScheduled() {
		return nil
	}
	if p.lastRevocationSync.Load().fresh(p.clock, maxStaleness) || p.sharedRevocationSync.Load().fresh(p.clock, maxStaleness) {
		return nil
	}
	return coreerrs.Wrap(ErrRevocationStale, "no revocation sync within staleness budget")
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
func (p *revocationChecker) initialRevocationSync(ctx context.Context) error {
	if p.storage == nil {
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
	waitCtx, cancel := context.WithTimeout(ctx, p.initialSyncWait)
	defer cancel()
	var lastErr error
	err := coreretry.Do(waitCtx, func(context.Context) error {
		lastErr = p.storage.Sync(ctx)
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
	err = boundaryError("initial revocation sync", 0, ErrRevocationCheck, err)
	if p.failOpen {
		p.logger.ErrorContext(ctx, "initial revocation sync failed; starting anyway (fail-open)", "error", err)
		// Start the staleness clock at construction, so the scheduled sync has
		// the whole bound to catch up before checks count as stale.
		p.markRevocationSynced()
		return nil
	}
	return err
}

// revocationItemFromVerifiedClaims extracts the revocation lookup key from
// signature-verified claims/header. The bool is false when the configured jti
// or kid is missing or not a non-empty string, so the caller decides whether to
// reject the token or fall back to the full token.
func revocationItemFromVerifiedClaims(itemType string, claims jwt.MapClaims, header map[string]any, token string) (string, bool) {
	switch itemType {
	case RevocationItemTypeJTI:
		jti, ok := claims[RevocationItemTypeJTI].(string)
		return jti, ok && jti != ""
	case RevocationItemTypeKID:
		kid, ok := header[RevocationItemTypeKID].(string)
		return kid, ok && kid != ""
	}
	return token, true
}

// wireRevocation prepares the revocation clients and storage and shares the
// Provider's HTTP client with them.
func (p *revocationChecker) wireRevocation(o *options) {
	p.prepareRevocationClients(o.discoveryAllowLoopbackHTTP)

	// Share the Provider's HTTP client with any revocation loader that
	// opts into injection by implementing SetHTTPClient(*http.Client).
	// Every OIDC outbound call (discovery, JWKS, introspection,
	// userinfo, revocation) then uses the same connection pool and
	// proxy resolver. Loaders decide their own preserve-vs-overwrite
	// policy in their SetHTTPClient implementation.
	if setter, ok := p.loader.(httpClientSetter); ok {
		setter.SetHTTPClient(p.client)
	}

	// Initialize revocation storage if not provided but filter/loader are available
	if p.storage == nil && o.revocationFilter != nil {
		p.storage = NewFilterRevocationStorage(o.revocationFilter, p.loader, o.revocationAuthoritative)
	} else if p.storage == nil && p.loader != nil && o.revocationFilter == nil {
		// Warn if loader is provided without filter - loader will be ignored
		p.logger.Warn("revocation loader provided without revocation filter, loader will be ignored")
	}

	// A ready-made storage (e.g. the factory's filter storage wrapping a
	// URLRevocationLoader) gets the shared client too, so its loader does
	// not fail every sync with ErrLoaderClientNotConfigured.
	if setter, ok := p.storage.(httpClientSetter); ok {
		setter.SetHTTPClient(p.client)
	}

	// The built-in storage is provider-local by now (created above or cloned
	// in prepareRevocationClients), so the setting never leaks to a storage
	// shared with another provider.
	if storage, ok := p.storage.(*filterRevocationStorage); ok && storage != nil {
		storage.allowEmpty = o.revocationAllowEmpty
	}
}

// httpClientSetter is implemented by a revocation loader or storage that
// accepts the Provider's shared HTTP client.
type httpClientSetter interface {
	SetHTTPClient(*http.Client)
}

// scheduledRevocationSync is the revocation sync job. A shared filter that a
// peer replica is rebuilding right now is skipped silently, but not counted
// as a sync of this provider: whether that rebuild publishes is unknown. A
// superseded rebuild (this one stalled past its lease) is reported as a
// failed sync, although the filter stays consistent. Each run is bounded by
// [WithRevocationSyncTimeout].
//
// Whatever the outcome, the run ends by reading the time of the last sync
// committed by any replica ([revocationChecker.refreshSharedRevocationSync]). That is
// what keeps a replica fresh that loses the rebuild lease on every tick, and
// only for as long as some peer does commit.
func (p *revocationChecker) scheduledRevocationSync(ctx context.Context) error {
	if !p.syncMu.TryLock() {
		return nil
	}
	defer p.syncMu.Unlock()
	return p.syncRevocation(ctx)
}

// startRevocationRecovery coalesces requests with each other and with cron.
func (p *revocationChecker) startRevocationRecovery() {
	if !p.syncMu.TryLock() {
		return
	}
	if p.backgroundCtx.Err() != nil {
		p.syncMu.Unlock()
		return
	}
	go func() {
		defer p.syncMu.Unlock()
		ctx := p.backgroundCtx
		if err := p.syncRevocation(ctx); err != nil && ctx.Err() == nil {
			p.logger.WarnContext(ctx, "revocation recovery failed", "error", err)
		}
	}()
}

// syncRevocation runs with syncMu held. The provider lifetime bounds both
// entry points and the configured timeout includes the peer freshness read.
func (p *revocationChecker) syncRevocation(ctx context.Context) error {
	if err := p.backgroundCtx.Err(); err != nil {
		return err
	}

	ctx, cancel := corecontext.WithMaxTimeout(ctx, p.syncTimeout)
	defer cancel()
	defer p.refreshSharedRevocationSync(ctx)
	err := p.storage.Sync(ctx)
	if err == nil {
		p.markRevocationSynced()
		return nil
	}
	if errors.Is(err, probfilter.ErrRebuildInProgress) {
		return nil
	}
	p.metrics.revocationCheckErrors.Inc()
	return boundaryError("sync revocation", 0, ErrRevocationCheck, err)
}

// syncScheduled reports whether the provider syncs its revocation
// storage on [WithRevocationSyncSchedule].
func (p *revocationChecker) syncScheduled() bool {
	return p.syncSchedule != "" && p.storage != nil
}
