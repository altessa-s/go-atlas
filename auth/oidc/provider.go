// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"sync/atomic"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/robfig/cron/v3"

	"github.com/altessa-s/go-atlas-core/runtime/cycle"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
	corectx "github.com/altessa-s/go-atlas-core/context"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	authjwt "github.com/altessa-s/go-atlas/auth/jwt"
	probfilter "github.com/altessa-s/go-atlas/cache/probfilter"
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
	// ErrDiscoveryValidation is returned by [Provider.getDiscoveryInfo] in
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
	opts                *options
	discoveryURL        string
	client              *http.Client
	discoveryInfo       *discoveryInfo
	jwks                keyfunc.Keyfunc
	keyResolver         authjwt.KeyResolver
	backgroundCtx       context.Context
	cancelBackgroundCtx context.CancelFunc
	logger              *slog.Logger
	tokenCache          Cacher
	revocationStorage   RevocationStorage
	revocationLoader    probfilter.DataLoader
	revocationFilter    probfilter.Filter

	// Internal state derived from options
	verifierOptions  *verifierOptions
	celCompiledRules []celPreCompiledValidationRule // Compiled CEL rules from verifierOptions.celRules

	// defaultPolicy compiles the default validation options (verifierOptions,
	// or the built-in defaults when none are set); builtinPolicy compiles the
	// built-in defaults, used to verify signatures before automatic preset
	// selection. Both are built in NewProvider once discovery and the JWKS
	// key resolver are known, and are the same object without default options.
	defaultPolicy *validationPolicy
	builtinPolicy *validationPolicy

	// lastKeyFingerprint memoizes the fingerprint of the most recently
	// fingerprinted signing key (see cachedKeyFingerprint).
	lastKeyFingerprint atomic.Pointer[keyFingerprintMemo]

	// jwksRefreshTask makes concurrent RefreshJWKS calls collapse into one.
	jwksRefreshTask *cycle.Guard
	// refreshCron runs the periodic JWKS refresh and revocation sync; nil
	// when no schedule is configured.
	refreshCron *cron.Cron

	// lastJWKSRefreshUnixNanos records when the locally cached JWKS was
	// last fully refreshed by the provider. It is read by
	// [Provider.checkJWKSStaleness] to enforce the
	// [WithJWKSMaxStaleness] freshness budget and is updated atomically
	// from every successful path that replaces the cache contents
	// (initial bootstrap in [Provider.initializeJWKS] and every
	// successful refreshJWKSInternal).
	lastJWKSRefreshUnixNanos atomic.Int64

	// lastRevocationSyncUnixNanos records the last successful revocation sync
	// (zero: none yet); see [Provider.checkRevocationStaleness].
	lastRevocationSyncUnixNanos atomic.Int64

	// sharedRevocationSyncUnixNanos caches, as an instant of the local clock,
	// when the last sync committed by any replica happened, derived from the
	// age the storage reported ([RevocationSyncReporter]) to a scheduled sync
	// job (zero: unknown); see
	// [Provider.refreshSharedRevocationSync].
	sharedRevocationSyncUnixNanos atomic.Int64

	// seenSharedRevocationCommit is the storage's own timestamp of the commit
	// sharedRevocationSyncUnixNanos was derived from (zero: none). It only
	// identifies the commit; it is never compared with the local clock.
	seenSharedRevocationCommit atomic.Int64

	// metrics provides Prometheus-compatible instrumentation for OIDC operations.
	metrics *oidcMetrics
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

	if err := applyServiceConfig(o); err != nil {
		return nil, err
	}

	client := o.httpClient
	if client == nil {
		return nil, ErrHTTPClientRequired
	}
	client = protectedHTTPClient(client, o.discoveryAllowLoopbackHTTP)

	// Create Provider from options
	p := &Provider{
		opts:              o,
		discoveryURL:      discoveryURL,
		client:            client,
		logger:            o.logger,
		tokenCache:        o.tokenCache,
		revocationStorage: o.revocationStorage,
		revocationFilter:  o.revocationFilter,
		revocationLoader:  o.revocationLoader,
		verifierOptions:   o.verifierOptions,
		metrics:           newOIDCMetrics(o.meter),
	}
	p.jwksRefreshTask = cycle.New(p.refreshJWKSInternal)

	p.wireRevocation(client)

	// Validate and process preset selection rules
	p.validatePresetSelectionRules()

	// Validate and compile CEL rules in verifier options
	if err := p.validateCELRules(); err != nil {
		return nil, err
	}

	if err := p.prepareDefaultVerifierOptions(); err != nil {
		return nil, err
	}

	// Background work (JWKS storage, the refresh cron) keeps ctx's values but
	// not its cancellation: a startup-timeout ctx must not silence every later
	// refresh. Only Close ends it.
	p.backgroundCtx, p.cancelBackgroundCtx = context.WithCancel(context.WithoutCancel(ctx))

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

	const discoveryTimeout = 5 * time.Second
	discoveryCtx, cancelFunc := corectx.ApplyTimeout(ctx, discoveryTimeout)
	defer cancelFunc()

	var err error
	if err = p.getDiscoveryInfo(discoveryCtx); err != nil {
		return nil, err
	}

	// Initialize JWKS with caching and automatic refresh
	if err = p.initializeJWKS(ctx); err != nil {
		return nil, err
	}

	// Compile presets only now: preset verifiers bind the discovery issuer
	// (fallback when a preset sets none) and the JWKS key resolver, both of
	// which are known only after discovery.
	if err = p.compilePresets(); err != nil {
		return nil, err
	}
	p.compileDefaultPolicies()

	if err = p.initialRevocationSync(ctx); err != nil {
		return nil, err
	}

	// Prepare the provider-owned refresh cron (JWKS refresh, revocation sync).
	if jobs := p.refreshJobs(); len(jobs) > 0 {
		if p.refreshCron, err = p.newRefreshCron(jobs); err != nil {
			return nil, err
		}
	}

	// Register with health coordinator if provided
	if o.healthCoordinator != nil {
		o.healthCoordinator.RegisterService("oidc", p)
	}

	constructed = true
	if p.refreshCron != nil {
		p.refreshCron.Start()
	}
	return p, nil
}

// applyServiceConfig loads the service config file set by
// WithServiceConfigPath, if any, and applies its validation options over the
// ones already in o.
func applyServiceConfig(o *options) error {
	if o.serviceConfigPath == "" {
		return nil
	}
	config, err := LoadServiceConfig(o.serviceConfigPath)
	if err != nil {
		return coreerrs.WrapOperation(err, "load service config")
	}

	// Convert service config to provider options
	configOpts, err := config.ToProviderOptions()
	if err != nil {
		return coreerrs.WrapOperation(err, "convert service config to options")
	}

	// Apply all validation options from config
	// This will override any previously set validation options
	for i := range configOpts {
		configOpts[i](o)
	}
	return nil
}

// wireRevocation prepares the revocation clients and storage and shares the
// Provider's HTTP client with them.
func (p *Provider) wireRevocation(client *http.Client) {
	p.prepareRevocationClients()

	// Share the Provider's HTTP client with any revocation loader that
	// opts into injection by implementing SetHTTPClient(*http.Client).
	// Every OIDC outbound call (discovery, JWKS, introspection,
	// userinfo, revocation) then uses the same connection pool and
	// proxy resolver. Loaders decide their own preserve-vs-overwrite
	// policy in their SetHTTPClient implementation.
	if setter, ok := p.revocationLoader.(httpClientSetter); ok {
		setter.SetHTTPClient(client)
	}

	// Initialize revocation storage if not provided but filter/loader are available
	if p.revocationStorage == nil && p.revocationFilter != nil {
		p.revocationStorage = NewFilterRevocationStorage(p.revocationFilter, p.revocationLoader, p.opts.revocationAuthoritative)
	} else if p.revocationStorage == nil && p.revocationLoader != nil && p.revocationFilter == nil {
		// Warn if loader is provided without filter - loader will be ignored
		p.logger.Warn("revocation loader provided without revocation filter, loader will be ignored")
	}

	// A ready-made storage (e.g. the factory's filter storage wrapping a
	// URLRevocationLoader) gets the shared client too, so its loader does
	// not fail every sync with ErrLoaderClientNotConfigured.
	if setter, ok := p.revocationStorage.(httpClientSetter); ok {
		setter.SetHTTPClient(client)
	}

	// The built-in storage is provider-local by now (created above or cloned
	// in prepareRevocationClients), so the setting never leaks to a storage
	// shared with another provider.
	if storage, ok := p.revocationStorage.(*filterRevocationStorage); ok && storage != nil {
		storage.allowEmpty = p.opts.revocationAllowEmpty
	}
}

// prepareDefaultVerifierOptions checks the default validation options' leeway
// and pre-builds their ignored-claims lookup set.
func (p *Provider) prepareDefaultVerifierOptions() error {
	if p.verifierOptions == nil {
		return nil
	}
	if err := checkLeeway(p.verifierOptions.leeway); err != nil {
		return coreerrs.Wrap(err, "default validation options")
	}
	p.verifierOptions.buildIgnoredSet()
	return nil
}

// UserinfoEndpoint returns the userinfo endpoint URL from OIDC discovery.
func (p *Provider) UserinfoEndpoint() string {
	if p.discoveryInfo == nil {
		return ""
	}
	return p.discoveryInfo.UserInfoURL
}

// TokenEndpoint returns the token endpoint URL from OIDC discovery.
func (p *Provider) TokenEndpoint() string {
	return p.discoveryInfo.TokenURL
}

// AuthorizationEndpoint returns the authorization endpoint URL from OIDC
// discovery, or "" when the IdP does not advertise one.
func (p *Provider) AuthorizationEndpoint() string {
	if p.discoveryInfo == nil {
		return ""
	}
	return p.discoveryInfo.AuthURL
}

// EndSessionEndpoint returns the RP-initiated logout endpoint
// (end_session_endpoint, OpenID Connect RP-Initiated Logout 1.0) from OIDC
// discovery, or "" when the IdP does not advertise one.
func (p *Provider) EndSessionEndpoint() string {
	if p.discoveryInfo == nil {
		return ""
	}
	return p.discoveryInfo.EndSessionURL
}

// RevocationEndpoint returns the token revocation endpoint
// (revocation_endpoint, RFC 7009) from OIDC discovery, or "" when the IdP
// does not advertise one.
func (p *Provider) RevocationEndpoint() string {
	if p.discoveryInfo == nil {
		return ""
	}
	return p.discoveryInfo.RevocationURL
}

// headerToMap converts a verified authjwt.Header into the map[string]any header
// shape the revocation and caching paths consume.
func headerToMap(hdr authjwt.Header) map[string]any {
	return map[string]any{"alg": hdr.Alg, "kid": hdr.Kid, "typ": hdr.Typ}
}

// toAuthAlgorithms converts a list of JWT method names into the [authjwt.Algorithm]
// allow-list the verifier expects.
func toAuthAlgorithms(methods []string) []authjwt.Algorithm {
	algs := make([]authjwt.Algorithm, len(methods))
	for i, m := range methods {
		algs[i] = authjwt.Algorithm(m)
	}
	return algs
}

// verifySignature verifies a JWT signature and validates its temporal claims
// when present, without binding audience / subject. It is used for signed
// userinfo responses, which legitimately omit exp — so exp is validated if
// present but not required (matching the prior signature-plus-temporal
// behavior). Leeway is zero to mirror the previous strict default. The issuer
// is bound when discovery resolved one, so a signed userinfo response from a
// different issuer sharing the JWKS is rejected.
func (p *Provider) verifySignature(ctx context.Context, token string) (map[string]any, error) {
	opts := []authjwt.Option{
		authjwt.WithAllowedAlgorithms(toAuthAlgorithms(DefaultValidMethods)...),
		authjwt.WithExpirationOptional(),
		authjwt.WithLeeway(0),
	}
	opts = coreslices.AppendIfFunc(opts, p.discoveryInfo != nil && p.discoveryInfo.Issuer != "", func() []authjwt.Option {
		return []authjwt.Option{authjwt.WithIssuer(p.discoveryInfo.Issuer)}
	})
	v := authjwt.NewVerifier(p.keyResolver, opts...)
	claims, err := v.Verify(ctx, token)
	if err != nil {
		return nil, err
	}
	return map[string]any(claims), nil
}

// algorithmAllowed reports whether alg is permitted by ops. An empty validMethods
// set falls back to DefaultValidMethods, mirroring [Provider.jwtVerifyOptions].
func algorithmAllowed(ops *verifierOptions, alg string) bool {
	methods := ops.validMethods
	if len(methods) == 0 {
		methods = DefaultValidMethods
	}
	return slices.Contains(methods, alg)
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
	if p.discoveryInfo != nil {
		issuer = p.discoveryInfo.Issuer
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

// checkValidatable rejects an empty token and, per [WithJWKSMaxStaleness], a
// stale key set. It runs before any (cached) signature verification so cached
// verifications do not bypass the staleness budget.
func (p *Provider) checkValidatable(ctx context.Context, token string) error {
	if token == "" {
		return coreerrs.Wrap(ErrTokenInvalid, "token is empty")
	}
	// See [Provider.checkJWKSStaleness] for the per-mode behavior.
	return p.checkJWKSStaleness(ctx)
}

// validateTokenWithOptions is the uninstrumented body of
// [Provider.ValidateTokenWithOptions].
func (p *Provider) validateTokenWithOptions(ctx context.Context, token string, opt ...ValidationOption) (map[string]any, error) {
	if err := p.checkValidatable(ctx, token); err != nil {
		return nil, err
	}

	// Fast path: no presets, no default options, and no overrides
	if len(opt) == 0 && p.verifierOptions == nil && len(p.opts.presetRules) == 0 {
		claims, header, err := p.parseAndValidateToken(ctx, token, p.defaultPolicy)
		if err != nil {
			return nil, err
		}
		if err := p.checkTokenRevocationVerified(ctx, token, claims, header); err != nil {
			return nil, err
		}
		return claims, nil
	}

	// No per-call options and preset selection rules configured: select the
	// preset from the signature-verified claims and validate those claims
	// with it, or with the default options when no registered preset applies.
	// The signature is verified once either way.
	if len(opt) == 0 && len(p.opts.presetRules) > 0 {
		claims, header, preset, err := p.verifyAndSelectPreset(ctx, token)
		if err != nil {
			return nil, err
		}
		policy, scope := p.defaultPolicy, ""
		if preset != nil {
			policy, scope = preset.policy, fmt.Sprintf(" by preset '%s'", preset.name)
		}
		if err := p.validateVerified(ctx, token, claims, header, policy, scope); err != nil {
			return nil, err
		}
		return claims, nil
	}

	// Without per-call overrides reuse the pre-compiled default policy;
	// otherwise compile the per-call options.
	policy := p.defaultPolicy
	if len(opt) > 0 {
		ops := cloneVerifierOptions(p.verifierOptions)
		applyValidationOptions(ops, opt...)
		ops.buildIgnoredSet()
		compiledCELRules, err := compileVerifierCELRules(ops)
		if err != nil {
			return nil, err
		}
		policy = p.newValidationPolicy(ops, compiledCELRules)
	}

	claims, header, err := p.parseAndValidateToken(ctx, token, policy)
	if err != nil {
		logValidationFailure(ctx, p.logger, "failed to validate token", err)
		return nil, err
	}

	if err := p.checkTokenRevocationVerified(ctx, token, claims, header); err != nil {
		return nil, err
	}

	return claims, nil
}

// verifyAndSelectPreset verifies token's signature against the built-in
// algorithm set — the preset is only known after inspecting the claims — and
// selects the preset for the verified claims. preset is nil when no rule
// matches or the matched preset is not registered.
//
// The selection rules see a deep copy of the claims, so a matcher that
// modifies its argument cannot change the claims that are validated,
// checked for revocation and returned.
func (p *Provider) verifyAndSelectPreset(ctx context.Context, token string) (jwt.MapClaims, map[string]any, *ValidationPreset, error) {
	claims, header, err := p.verifyTokenSignature(ctx, token, p.builtinPolicy)
	if err != nil {
		err = coreerrs.Wrapf(ErrTokenInvalid, "signature verification failed: %v", err)
		logValidationFailure(ctx, p.logger, "signature verification failed", err)
		return nil, nil, nil, err
	}

	presetName := p.selectPresetForClaims(cloneClaims(claims))
	if presetName == "" {
		return claims, header, nil, nil
	}
	preset := p.getPreset(presetName)
	if preset == nil {
		p.logger.WarnContext(ctx, "preset rule matched but preset not found, using default validation",
			"preset_name", presetName)
	}
	return claims, header, preset, nil
}

// validateVerified validates signature-verified claims against policy: the
// policy's signing-algorithm allow-list on the verified header (the signature
// was verified against the built-in set), the claim policy, then revocation.
// scope qualifies the algorithm error (e.g. " by preset 'x'").
func (p *Provider) validateVerified(
	ctx context.Context,
	token string,
	claims jwt.MapClaims,
	header map[string]any,
	policy *validationPolicy,
	scope string,
) error {
	alg, _ := header["alg"].(string)
	if !algorithmAllowed(policy.ops, alg) {
		return coreerrs.Wrapf(ErrTokenInvalid, "signing algorithm %q not allowed%s", alg, scope)
	}
	if err := p.applyPolicy(claims, policy); err != nil {
		logValidationFailure(ctx, p.logger, "failed to validate token", err)
		return err
	}
	return p.checkTokenRevocationVerified(ctx, token, claims, header)
}

// cloneClaims deep-copies a claim set: nested objects and arrays are copied,
// scalar values (immutable in decoded JSON) are shared.
func cloneClaims(claims map[string]any) map[string]any {
	out := make(map[string]any, len(claims))
	for k, v := range claims {
		out[k] = cloneClaimValue(v)
	}
	return out
}

func cloneClaimValue(v any) any {
	switch tv := v.(type) {
	case map[string]any:
		return cloneClaims(tv)
	case []any:
		out := make([]any, len(tv))
		for i, e := range tv {
			out[i] = cloneClaimValue(e)
		}
		return out
	default:
		return v
	}
}

// jwtVerifyOptions converts verifier options to authjwt verifier options. It
// maps only the registered-claim knobs authjwt enforces (algorithms, leeway,
// exp/nbf/iat, issuer, subject, audience); the OIDC-specific checks
// (required/expected/ignored claims, scopes, authorized party, CEL rules) stay
// in [Provider.validateClaims]. Claims validation is toggled at the call site
// via VerifySignature vs VerifyWithHeader, so withoutClaimsValidation is not
// translated here.
func (p *Provider) jwtVerifyOptions(ops *verifierOptions) []authjwt.Option {
	if ops == nil {
		return nil
	}

	// Restrict allowed signing algorithms to prevent algorithm confusion attacks.
	// Use configured methods if set, otherwise fall back to DefaultValidMethods.
	validMethods := ops.validMethods
	if len(validMethods) == 0 {
		validMethods = DefaultValidMethods
	}
	verifyOpts := []authjwt.Option{authjwt.WithAllowedAlgorithms(toAuthAlgorithms(validMethods)...)}

	// Clock-skew tolerance. Passed unconditionally so ops.leeway == 0 yields
	// strict, no-skew temporal validation (authjwt's WithLeeway accepts zero)
	// instead of falling back to authjwt's 30s default.
	verifyOpts = append(verifyOpts, authjwt.WithLeeway(ops.leeway))

	// The exp claim is always required: no validation option can accept a
	// token that never expires.
	verifyOpts = append(verifyOpts, authjwt.WithExpirationRequired())

	// Same contract for the nbf claim — `verify_not_before: true` in the
	// service config translates to "nbf must be present and respected".
	verifyOpts = coreslices.AppendIf(verifyOpts, ops.notBeforeRequired, authjwt.WithNotBeforeRequired())

	// Verify issued-at claim if requested
	verifyOpts = coreslices.AppendIf(verifyOpts, ops.issuedAt, authjwt.WithIssuedAt())

	// Always verify issuer (use discovery issuer as fallback)
	iss := ops.issuer
	if iss == "" && p.discoveryInfo != nil {
		iss = p.discoveryInfo.Issuer
	}
	verifyOpts = coreslices.AppendIf(verifyOpts, iss != "", authjwt.WithIssuer(iss))

	// Verify subject if specified
	verifyOpts = coreslices.AppendIf(verifyOpts, ops.subject != "", authjwt.WithSubject(ops.subject))

	// Verify audience if specified
	verifyOpts = coreslices.AppendIf(verifyOpts, len(ops.audience) > 0, authjwt.WithAudiences(ops.audience...))

	return verifyOpts
}

// parseAndValidateToken verifies token's signature (through the token cache)
// and validates its claims against policy. Returns the verified claims and
// token header.
func (p *Provider) parseAndValidateToken(ctx context.Context, token string, policy *validationPolicy) (jwt.MapClaims, map[string]any, error) {
	claims, header, err := p.verifyTokenSignature(ctx, token, policy)
	if err != nil {
		return nil, nil, coreerrs.Wrapf(ErrTokenInvalid, "%s", err)
	}
	if err := p.applyPolicy(claims, policy); err != nil {
		return nil, nil, err
	}
	return claims, header, nil
}

func cloneVerifierOptions(base *verifierOptions) *verifierOptions {
	if base == nil {
		return defaultVerifierOptions()
	}

	cloned := *base

	cloned.audience = slices.Clone(base.audience)
	cloned.requiredClaims = slices.Clone(base.requiredClaims)
	cloned.ignoredClaims = slices.Clone(base.ignoredClaims)
	cloned.allowedClientIDs = slices.Clone(base.allowedClientIDs)
	cloned.requiredScopes = slices.Clone(base.requiredScopes)
	cloned.allowedAuthorizedParties = slices.Clone(base.allowedAuthorizedParties)
	cloned.celRules = slices.Clone(base.celRules)
	cloned.validMethods = slices.Clone(base.validMethods)
	cloned.expectedClaims = nil

	if base.expectedClaims != nil {
		cloned.expectedClaims = maps.Clone(base.expectedClaims)
	}

	// Re-build the cached set since ignoredClaims may have been modified.
	cloned.buildIgnoredSet()

	return &cloned
}

func applyValidationOptions(ops *verifierOptions, options ...ValidationOption) {
	for _, opt := range options {
		opt(ops)
	}
}

// httpClientSetter is implemented by a revocation loader or storage that
// accepts the Provider's shared HTTP client.
type httpClientSetter interface {
	SetHTTPClient(*http.Client)
}
