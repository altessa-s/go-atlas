// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/altessa-s/go-atlas-core/runtime/cycle"

	corectx "github.com/altessa-s/go-atlas-core/context"
)

var (
	ErrTokenInvalid              = errors.New("invalid token")
	ErrCELValidation             = errors.New("CEL validation failed")
	ErrDiscovery                 = errors.New("discovery failed")
	ErrIntrospection             = errors.New("introspection failed")
	ErrTokenRevoked              = errors.New("token has been revoked")
	ErrAudienceNotConfigured     = errors.New("no expected audience configured")
	ErrLoaderClientNotConfigured = errors.New("revocation loader: HTTP client not configured")

	// ErrHTTPClientRequired is returned by [NewProvider] when no client is
	// passed with [WithHTTPClient]. The provider does not pick a transport
	// policy itself: pass the resilient, SSRF-protected client from
	// net/http/client (httpclient.New()), as the factory does.
	ErrHTTPClientRequired = errors.New("oidc: HTTP client is required")
	// ErrFilterNotRebuildable is returned by [filterRevocationStorage.Sync]
	// when the underlying filter does not implement [probfilter.RebuildableFilter].
	ErrFilterNotRebuildable = errors.New("revocation filter does not support rebuild/sync")
	// ErrRevocationLoadFailed is returned by [URLRevocationLoader] and
	// related sources when the upstream signals a non-OK status, the body
	// exceeds the size cap, or an entry exceeds [MaxRevocationEntryBytes], and
	// by the filter storage's Sync when the source turns up empty while the
	// current revocation set is not. The current set is kept in every case.
	ErrRevocationLoadFailed = errors.New("revocation source load failed")
	// ErrJWKSStale is returned when the time since the last successful JWKS
	// refresh exceeds the configured [DefaultJWKSMaxStaleness] (or whatever
	// the operator passed via [WithJWKSMaxStaleness]) and the failure mode
	// is [JWKSFailureModeEnforce]. It signals that the cached key set may
	// have missed an IdP-side rotation and that any cached/locally-verifiable
	// token must no longer be trusted.
	ErrJWKSStale = errors.New("JWKS cache too stale to trust")
	// ErrDiscoveryValidation is returned by [discoveryClient.getDiscoveryInfo] in
	// [DiscoveryValidationModeEnforce] when the discovery document's issuer
	// does not match the configured issuer, or when an advertised endpoint is
	// non-HTTPS or does not share the issuer's host. It signals a possible
	// mix-up / SSRF attempt via a malicious or compromised discovery response.
	ErrDiscoveryValidation = errors.New("discovery document failed issuer/endpoint validation")
	// ErrRedirectNotAllowed is returned when an outbound provider call
	// (discovery, JWKS, introspection, userinfo, revocation list) is
	// redirected to a different origin or to a non-HTTPS URL. It signals a
	// possible attempt to substitute attacker-controlled keys or to exfiltrate
	// credentials through an open redirect on the IdP.
	ErrRedirectNotAllowed = errors.New("redirect to a different origin or to non-HTTPS is not allowed")
	// ErrJWKSTooLarge is returned when the JWKS document exceeds 1 MiB or
	// holds more than 100 keys. The cached key set is left unchanged.
	ErrJWKSTooLarge = errors.New("JWKS document too large")
	// ErrRevocationCheck is returned when the revocation storage cannot answer
	// (IsRevoked or the initial Sync fails) and the provider is fail-closed —
	// the default unless [WithRevocationFailOpen] is set.
	ErrRevocationCheck = errors.New("revocation check failed")
	// ErrRevocationReadinessLost signals that a storage needs a new Sync
	// before it can answer lookups. The provider schedules one recovery.
	ErrRevocationReadinessLost = errors.New("revocation storage lost readiness")
	// ErrRevocationStale is wrapped by [ErrRevocationCheck] when the revocation
	// storage has not completed a sync within [WithRevocationMaxStaleness].
	ErrRevocationStale = errors.New("revocation data too stale to trust")
)

// maxDiscoveryResponseSize is the maximum bytes read from the OIDC discovery
// endpoint. Discovery documents are a few KB in practice; anything larger
// indicates a misbehaving or malicious IdP.
const maxDiscoveryResponseSize = 1 << 20 // 1 MiB

// maxJWKSResponseSize is the maximum bytes read from the JWKS endpoint. Real
// key sets are a few KB; a larger document is rejected with [ErrJWKSTooLarge].
const maxJWKSResponseSize = 1 << 20 // 1 MiB

// maxJWKSKeys is the maximum number of keys accepted in a JWK Set. IdPs
// publish a handful of keys (current, next, previous); a set with more keys is
// rejected with [ErrJWKSTooLarge].
const maxJWKSKeys = 100

// maxDrainBytes bounds how much of an unread response body drainAndClose
// consumes to keep the HTTP connection reusable. A body larger than this is
// cheaper to abandon (closing the connection) than to stream to completion.
const maxDrainBytes = 1 << 16 // 64 KiB

// drainAndClose drains and closes an HTTP response body.
// This ensures the connection can be reused by the HTTP client.
// The drain is bounded by maxDrainBytes so an unbounded upstream body cannot
// force the client to stream it to completion.
func drainAndClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxDrainBytes)) //nolint:errcheck
	_ = resp.Body.Close()                                                //nolint:errcheck
}

// Provider handles OIDC token validation with automatic JWKS management.
type Provider struct {
	discovery  discoveryClient
	keys       jwksKeySource
	policy     policyEvaluator
	revocation revocationChecker
	userInfo   userInfoClient

	backgroundCtx       context.Context
	cancelBackgroundCtx context.CancelFunc
	logger              *slog.Logger
	metrics             *oidcMetrics
	ownedHTTPClient     *http.Client
	refreshCron         *cron.Cron
	closeOnce           sync.Once
	healthMu            sync.Mutex
	unregisterHealth    func()
}

// NewProvider creates an OIDC provider from a discovery URL. A client passed
// with [WithHTTPClient] is required; NewProvider returns
// [ErrHTTPClientRequired] without one.
//
// ctx bounds only the work done before NewProvider returns: discovery, the
// initial JWKS fetch and the initial revocation sync. The background work
// ([WithJWKSRefreshSchedule], [WithRevocationSyncSchedule]) keeps ctx's values
// but outlives its cancellation, so the provider must be closed: only
// [Provider.Close] stops it.
//
// When a revocation storage is configured (directly, or assembled from
// [WithRevocationFilter] and [WithRevocationLoader]), NewProvider runs one
// [RevocationStorage.Sync] before returning so the provider never serves with
// an unpopulated revocation filter. A failed initial sync fails construction
// with [ErrRevocationCheck] unless [WithRevocationFailOpen] is set, in which
// case it is logged and the provider starts anyway.
//
// Example:
//
//	provider, _ := oidc.NewProvider(ctx, "https://example.com/.well-known/openid-configuration",
//		oidc.WithHTTPClient(httpclient.New()))
//	defer provider.Close()
func NewProvider(ctx context.Context, discoveryURL string, opt ...Option) (*Provider, error) {
	// Build options first
	o := newOptions(opt...)

	p := &Provider{logger: o.logger, metrics: newOIDCMetrics(o.meter)}
	lifetime, cancel := context.WithCancel(context.WithoutCancel(ctx))
	p.backgroundCtx, p.cancelBackgroundCtx = lifetime, cancel
	p.discovery = discoveryClient{
		discoveryURL:               discoveryURL,
		logger:                     o.logger,
		discoveryValidationMode:    o.discoveryValidationMode,
		discoveryAllowLoopbackHTTP: o.discoveryAllowLoopbackHTTP,
	}
	p.revocation = newRevocationChecker(o, lifetime, p.metrics)

	// Every failure from here on must release what construction acquired:
	// the background context (JWKS storage, keyfunc) and the HTTP client's
	// idle connections. The refresh cron is started only once construction
	// has succeeded, so a failed construction never leaves it running.
	constructed := false
	defer func() {
		if !constructed {
			p.Close()
		}
	}()

	client := o.httpClient
	if client == nil {
		return nil, ErrHTTPClientRequired
	}
	client = protectedHTTPClient(client, o.discoveryAllowLoopbackHTTP)
	if o.httpClientOwned {
		p.ownedHTTPClient = client
	}

	p.discovery.client = client
	p.revocation.client = client
	p.revocation.wireRevocation(o)
	p.keys = jwksKeySource{
		client:           client,
		logger:           o.logger,
		metrics:          p.metrics,
		backgroundCtx:    p.backgroundCtx,
		jwksHTTPTimeout:  o.jwksHTTPTimeout,
		jwksMaxStaleness: o.jwksMaxStaleness,
		jwksFailureMode:  o.jwksFailureMode,
	}
	if o.jwksRefreshEnabled {
		p.keys.schedule = o.jwksRefreshSchedule
	}
	p.keys.jwksRefreshTask = cycle.New(p.keys.refreshJWKSInternal)
	p.policy = policyEvaluator{
		keys:                 &p.keys,
		logger:               o.logger,
		metrics:              p.metrics,
		tokenCache:           o.tokenCache,
		tokensCacheKeyPrefix: o.tokensCacheKeyPrefix,
		audienceFailureMode:  o.audienceFailureMode,
		verifierOptions:      o.verifierOptions,
		presets:              o.presets,
		presetRules:          o.presetRules,
	}

	// Validate and process preset selection rules
	p.policy.validatePresetSelectionRules()

	// Validate and compile CEL rules in verifier options
	if err := p.policy.validateCELRules(); err != nil {
		return nil, err
	}

	if err := p.policy.prepareDefaultVerifierOptions(); err != nil {
		return nil, err
	}

	const discoveryTimeout = 5 * time.Second
	discoveryCtx, cancelFunc := corectx.ApplyTimeout(ctx, discoveryTimeout)
	defer cancelFunc()

	stopDiscoveryTimer := p.metrics.discoveryDuration.Start()
	err := p.discovery.getDiscoveryInfo(discoveryCtx)
	stopDiscoveryTimer()
	if err != nil {
		return nil, err
	}

	p.keys.info = p.discovery.info
	p.policy.info = p.discovery.info
	p.revocation.info = p.discovery.info
	p.userInfo = userInfoClient{client: client, info: p.discovery.info, keys: &p.keys}

	// Initialize JWKS with caching and automatic refresh
	if err = p.keys.initializeJWKS(ctx); err != nil {
		return nil, err
	}

	// Compile presets only now: preset verifiers bind the discovery issuer
	// (fallback when a preset sets none) and the JWKS key resolver, both of
	// which are known only after discovery.
	if err = p.policy.compilePresets(); err != nil {
		return nil, err
	}
	p.policy.compileDefaultPolicies()

	if err = p.revocation.initialRevocationSync(ctx); err != nil {
		return nil, err
	}

	// Prepare the provider-owned refresh cron (JWKS refresh, revocation sync).
	if jobs := p.refreshJobs(); len(jobs) > 0 {
		if p.refreshCron, err = p.newRefreshCron(jobs); err != nil {
			return nil, err
		}
	}

	constructed = true
	if p.refreshCron != nil {
		p.refreshCron.Start()
	}
	if o.healthCoordinator != nil {
		p.registerHealth(o.healthCoordinator)
	}
	return p, nil
}

// UserinfoEndpoint returns the userinfo endpoint URL from OIDC discovery.
func (p *Provider) UserinfoEndpoint() string {
	if p.discovery.info == nil {
		return ""
	}
	return p.discovery.info.UserInfoURL
}

// TokenEndpoint returns the token endpoint URL from OIDC discovery.
func (p *Provider) TokenEndpoint() string {
	if p.discovery.info == nil {
		return ""
	}
	return p.discovery.info.TokenURL
}

// AuthorizationEndpoint returns the authorization endpoint URL from OIDC
// discovery, or "" when the IdP does not advertise one.
func (p *Provider) AuthorizationEndpoint() string {
	if p.discovery.info == nil {
		return ""
	}
	return p.discovery.info.AuthURL
}

// EndSessionEndpoint returns the RP-initiated logout endpoint
// (end_session_endpoint, OpenID Connect RP-Initiated Logout 1.0) from OIDC
// discovery, or "" when the IdP does not advertise one.
func (p *Provider) EndSessionEndpoint() string {
	if p.discovery.info == nil {
		return ""
	}
	return p.discovery.info.EndSessionURL
}

// RevocationEndpoint returns the token revocation endpoint
// (revocation_endpoint, RFC 7009) from OIDC discovery, or "" when the IdP
// does not advertise one.
func (p *Provider) RevocationEndpoint() string {
	if p.discovery.info == nil {
		return ""
	}
	return p.discovery.info.RevocationURL
}

// ValidateToken validates a JWT and returns its claims using default options.
func (p *Provider) ValidateToken(ctx context.Context, token string) (map[string]any, error) {
	return p.ValidateTokenWithOptions(ctx, token)
}

// ValidateTokenWithOptions validates a JWT with custom validation options.
func (p *Provider) ValidateTokenWithOptions(ctx context.Context, token string, opt ...ValidationOption) (map[string]any, error) {
	return p.observeValidation(func() (map[string]any, error) {
		return p.validateTokenWithOptions(ctx, token, opt...)
	})
}

// observeValidation records one validation attempt — the attempt counter,
// the duration and, on failure, the error counter — around validate. Every
// public ValidateToken* entry point goes through it exactly once.
func (p *Provider) observeValidation(validate func() (map[string]any, error)) (map[string]any, error) {
	issuer := ""
	if p.discovery.info != nil {
		issuer = p.discovery.info.Issuer
	}
	counters := p.metrics.forIssuer(issuer)
	counters.validations.Inc()
	stop := p.metrics.validationDuration.Start()
	defer stop()

	claims, err := validate()
	if err != nil {
		counters.errors.Inc()
	}
	return claims, err
}

// validateTokenWithOptions checks revocation only after the selected policy
// has accepted both the signature and the claims.
func (p *Provider) validateTokenWithOptions(ctx context.Context, token string, opt ...ValidationOption) (map[string]any, error) {
	claims, header, err := p.policy.evaluate(ctx, token, opt...)
	if err != nil {
		return nil, err
	}
	if err := p.revocation.checkTokenRevocationVerified(ctx, token, claims, header); err != nil {
		return nil, err
	}
	return claims, nil
}
