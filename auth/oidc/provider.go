// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/MicahParks/jwkset"
	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"

	"github.com/altessa-s/go-atlas/observability/metrics"

	corectx "github.com/altessa-s/go-atlas/core/context"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
	httpclient "github.com/altessa-s/go-atlas/transport/http/client"
)

var (
	ErrInvalidToken              = errors.New("invalid token")
	ErrCELValidation             = errors.New("CEL validation failed")
	ErrDiscovery                 = errors.New("discovery failed")
	ErrIntrospection             = errors.New("introspection failed")
	ErrTokenRevoked              = errors.New("token has been revoked")
	ErrAudienceNotConfigured     = errors.New("no expected audience configured")
	ErrSchedulerManaged          = errors.New("function is managed by scheduler, direct calls not allowed")
	ErrLoaderClientNotConfigured = errors.New("revocation loader: HTTP client not configured")
	// ErrFilterNotRebuildable is returned by [filterRevocationStorage.Sync]
	// when the underlying filter does not implement [RebuildableFilter].
	ErrFilterNotRebuildable = errors.New("revocation filter does not support rebuild/sync")
	// ErrRevocationLoadFailed is returned by [URLRevocationLoader] and
	// related sources when the upstream signals a non-OK status.
	ErrRevocationLoadFailed = errors.New("revocation source load failed")
	// ErrJWKSStale is returned when the time since the last successful JWKS
	// refresh exceeds the configured [DefaultJWKSMaxStaleness] (or whatever
	// the operator passed via [WithJWKSMaxStaleness]) and the failure mode
	// is [JWKSFailureModeEnforce]. It signals that the cached key set may
	// have missed an IdP-side rotation and that any cached/locally-verifiable
	// token must no longer be trusted.
	ErrJWKSStale = errors.New("JWKS cache too stale to trust")
)

// drainAndClose drains and closes an HTTP response body.
// This ensures the connection can be reused by the HTTP client.
func drainAndClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body) //nolint:errcheck
	_ = resp.Body.Close()                 //nolint:errcheck
}

type discoveryInfo struct {
	Issuer           string   `json:"issuer"`
	AuthURL          string   `json:"authorization_endpoint"`
	TokenURL         string   `json:"token_endpoint"`
	JwksURL          string   `json:"jwks_uri"`
	UserInfoURL      string   `json:"userinfo_endpoint"`
	IntrospectionURL string   `json:"introspection_endpoint"`
	Algorithms       []string `json:"id_token_signing_alg_values_supported"`
}

func (d *discoveryInfo) IsValid() bool {
	return d.Issuer != "" &&
		d.AuthURL != "" &&
		d.TokenURL != "" &&
		d.JwksURL != "" &&
		d.UserInfoURL != "" &&
		len(d.Algorithms) > 0
}

// Provider handles OIDC token validation with automatic JWKS management.
type Provider struct {
	opts                *options
	discoveryURL        string
	client              *http.Client
	discoveryInfo       *discoveryInfo
	jwks                keyfunc.Keyfunc
	backgroundCtx       context.Context
	cancelBackgroundCtx context.CancelFunc
	logger              *slog.Logger
	tokenCache          Cacher
	revocationStorage   RevocationStorage
	revocationLoader    DataLoader
	revocationFilter    Filter

	// Internal state derived from options
	verifierOptions  *verifierOptions
	celCompiledRules []celPreCompiledValidationRule // Compiled CEL rules from verifierOptions.celRules

	// Scheduler configuration
	scheduler          corescheduler.TaskRegistrar
	jwksRefreshRunning atomic.Bool // Guards against concurrent RefreshJWKS calls.

	schedulerJWKSRefreshRegistered atomic.Bool // Marks if RefreshJWKS is managed by scheduler.

	// lastJWKSRefreshUnixNanos records when the locally cached JWKS was
	// last fully refreshed by the provider. It is read by
	// [Provider.checkJWKSStaleness] to enforce the
	// [WithJWKSMaxStaleness] freshness budget and is updated atomically
	// from every successful path that replaces the cache contents
	// (initial bootstrap in [Provider.initializeJWKS] and every
	// successful refreshJWKSInternal).
	lastJWKSRefreshUnixNanos atomic.Int64

	// metrics provides Prometheus-compatible instrumentation for OIDC operations.
	metrics *oidcMetrics
}

// NewProvider creates an OIDC provider from a discovery URL.
// Call Close when done to release resources.
//
// Example:
//
//	provider, _ := oidc.NewProvider(ctx, "https://example.com/.well-known/openid-configuration")
//	defer provider.Close()
func NewProvider(ctx context.Context, discoveryURL string, opt ...Option) (*Provider, error) {
	// Build options first
	o := newOptions(opt...)

	// If service config path is set, load and apply the configuration
	if o.serviceConfigPath != "" {
		config, err := LoadServiceConfig(o.serviceConfigPath)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "load service config")
		}

		// Convert service config to provider options
		configOpts, err := config.ToProviderOptions()
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "convert service config to options")
		}

		// Apply all validation options from config
		// This will override any previously set validation options
		for i := range configOpts {
			configOpts[i](o)
		}
	}

	// Build the outbound HTTP client from caller-supplied options. Empty
	// httpClientOptions yields the resilient default of httpclient.New
	// (pooled transport, retry, breaker, env-proxy). Override per
	// deployment via WithHTTPClientOptions(httpclient.WithRetryMax(0))
	// and friends.
	client := httpclient.New(o.httpClientOptions...)

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
		scheduler:         o.scheduler,
		metrics:           newOIDCMetrics(o.collector),
	}

	// Share the Provider's HTTP client with any revocation loader that
	// opts into injection by implementing [httpclient.HTTPClientSetter].
	// Every OIDC outbound call (discovery, JWKS, introspection,
	// userinfo, revocation) then uses the same connection pool and
	// proxy resolver. Loaders decide their own preserve-vs-overwrite
	// policy in their SetHTTPClient implementation.
	if setter, ok := p.revocationLoader.(httpclient.HTTPClientSetter); ok {
		setter.SetHTTPClient(client)
	}

	// Initialize revocation storage if not provided but filter/loader are available
	if p.revocationStorage == nil && p.revocationFilter != nil {
		p.revocationStorage = NewFilterRevocationStorage(p.revocationFilter, p.revocationLoader)
	} else if p.revocationStorage == nil && p.revocationLoader != nil && p.revocationFilter == nil {
		// Warn if loader is provided without filter - loader will be ignored
		p.logger.Warn("revocation loader provided without revocation filter, loader will be ignored")
	}

	// Validate and process preset selection rules
	p.validatePresetSelectionRules()

	// Compile all presets (apply options and compile CEL rules)
	p.compilePresets()

	// Validate and compile CEL rules in verifier options
	p.validateCELRules()

	// Pre-build the ignored claims lookup set for the default verifier options.
	if p.verifierOptions != nil {
		p.verifierOptions.buildIgnoredSet()
	}

	p.backgroundCtx, p.cancelBackgroundCtx = context.WithCancel(ctx)

	const discoveryTimeout = 5 * time.Second
	discoveryCtx, cancelFunc := corectx.ApplyTimeout(p.backgroundCtx, discoveryTimeout)
	defer cancelFunc()

	var err error
	if err = p.getDiscoveryInfo(discoveryCtx); err != nil { //nolint:contextcheck // intentionally uses background context with timeout for discovery
		return nil, err
	}

	// Initialize JWKS with caching and automatic refresh
	if err = p.initializeJWKS(); err != nil {
		return nil, err
	}

	// Register background tasks with scheduler if provided
	if err = p.registerSchedulerTasks(o); err != nil { //nolint:contextcheck // registration uses background context internally
		return nil, coreerrs.WrapOperation(err, "register scheduler tasks")
	}

	// Register with health coordinator if provided
	if o.healthCoordinator != nil {
		o.healthCoordinator.RegisterService("oidc", p)
	}

	return p, nil
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

// parseToken parses a JWT and verifies its signature, returning the claims
// and the verified token header. Valid signing methods are always enforced
// (DefaultValidMethods) to prevent algorithm confusion attacks. Optional
// parser options can customize other behavior.
func (p *Provider) parseToken(token string, opts ...jwt.ParserOption) (jwt.MapClaims, map[string]any, error) {
	// Prepend valid methods restriction so it applies to all parse paths.
	// Callers can override by passing their own jwt.WithValidMethods (last wins).
	opts = append([]jwt.ParserOption{jwt.WithValidMethods(DefaultValidMethods)}, opts...)

	jwtToken, err := jwt.Parse(token, p.jwks.Keyfunc, opts...)
	if err != nil {
		return nil, nil, err
	}

	if !jwtToken.Valid {
		return nil, nil, coreerrs.Wrap(ErrInvalidToken, "token is invalid")
	}

	claims, ok := jwtToken.Claims.(jwt.MapClaims)
	if !ok {
		return nil, nil, coreerrs.Wrap(ErrInvalidToken, "invalid claims type")
	}

	return claims, jwtToken.Header, nil
}

// verifySignature verifies JWT signature and returns parsed claims.
func (p *Provider) verifySignature(token string) (map[string]any, error) {
	claims, _, err := p.parseToken(token)
	return claims, err
}

// parseTokenWithoutClaimsValidation parses a JWT without validating claims.
// Returns claims and the verified token header.
func (p *Provider) parseTokenWithoutClaimsValidation(token string) (jwt.MapClaims, map[string]any, error) {
	return p.parseToken(token, jwt.WithoutClaimsValidation())
}

// ValidateToken validates a JWT and returns its claims using default options.
func (p *Provider) ValidateToken(ctx context.Context, token string) (map[string]any, error) {
	return p.ValidateTokenWithOptions(ctx, token)
}

// ValidateTokenWithOptions validates a JWT with custom validation options.
func (p *Provider) ValidateTokenWithOptions(ctx context.Context, token string, opt ...ValidationOption) (map[string]any, error) {
	issuer := ""
	if p.discoveryInfo != nil {
		issuer = p.discoveryInfo.Issuer
	}
	issuerLabels := metrics.Labels{"issuer": issuer}
	p.metrics.tokenValidations.WithLabels(issuerLabels).Inc()
	stop := p.metrics.validationDuration.Start()
	defer stop()

	// Reject empty tokens immediately
	if token == "" {
		p.metrics.validationErrors.WithLabels(issuerLabels).Inc()
		return nil, coreerrs.Wrap(ErrInvalidToken, "token is empty")
	}

	// Refuse to validate against a stale key set when the operator has
	// opted in via WithJWKSMaxStaleness — see [Provider.checkJWKSStaleness]
	// for the per-mode behavior. Runs first so cached claims and
	// introspection short-circuits do not bypass the staleness budget.
	if err := p.checkJWKSStaleness(ctx); err != nil {
		p.metrics.validationErrors.WithLabels(issuerLabels).Inc()
		return nil, err
	}

	if err := p.checkTokenRevocation(ctx, token); err != nil {
		p.metrics.validationErrors.WithLabels(issuerLabels).Inc()
		return nil, err
	}

	// Try to get cached claims if caching is enabled (only after revocation check)
	if p.tokenCache != nil {
		cacheKey := tokenCacheKey(p.opts.tokensCacheKeyPrefix, token)
		var claims map[string]any
		if err := p.tokenCache.Get(ctx, cacheKey, &claims); err == nil {
			p.metrics.cacheHits.Inc()
			// Re-run the post-verification revocation check against cached
			// claims so that a token revoked after caching is still rejected
			// for the remainder of its TTL.
			if err := p.checkTokenRevocationVerifiedCached(ctx, token, claims); err != nil {
				p.metrics.validationErrors.WithLabels(issuerLabels).Inc()
				return nil, err
			}
			return claims, nil
		}
		p.metrics.cacheMisses.Inc()
		// Cache miss or error - fall through to validation
	}

	var presetClaims jwt.MapClaims
	var presetHeader map[string]any
	var presetVerifier *verifierOptions
	var presetCELRules []celPreCompiledValidationRule

	// Fast path: no presets, no default options, and no overrides
	if len(opt) == 0 && p.verifierOptions == nil && len(p.opts.presetRules) == 0 {
		claims, header, err := p.parseAndValidateToken(token, &verifierOptions{}, nil)
		if err != nil {
			p.metrics.validationErrors.WithLabels(issuerLabels).Inc()
			return nil, err
		}
		if err := p.finalizeValidatedToken(ctx, token, claims, header); err != nil {
			p.metrics.validationErrors.WithLabels(issuerLabels).Inc()
			return nil, err
		}
		return claims, nil
	}

	// If no options provided and preset selection rules are configured,
	// try automatic preset selection
	if len(opt) == 0 && len(p.opts.presetRules) > 0 {
		// Step 1: Verify signature FIRST for security (without claim validation)
		claimsForPreset, headerForPreset, err := p.parseTokenWithoutClaimsValidation(token)
		if err != nil {
			p.metrics.validationErrors.WithLabels(issuerLabels).Inc()
			p.logger.ErrorContext(ctx, "signature verification failed", slog.Any("error", err))
			return nil, coreerrs.Wrapf(ErrInvalidToken, "signature verification failed: %v", err)
		}

		// Step 2: Select preset based on verified claims
		presetName := p.selectPresetForClaims(claimsForPreset)

		// Step 3: If preset matched, use pre-compiled verifier options
		if presetName != "" {
			preset := p.getPreset(presetName)
			if preset != nil && preset.compiledVerifier != nil {
				presetClaims = claimsForPreset
				presetHeader = headerForPreset
				presetVerifier = preset.compiledVerifier
				presetCELRules = preset.compiledCELRules
			} else if preset == nil {
				// Preset rule matched but preset not registered - log warning
				p.logger.WarnContext(ctx, "preset rule matched but preset not found, using default validation",
					"preset_name", presetName)
			}
		}
		// No preset matched or preset not found - fall through to default validation
	}

	if presetVerifier != nil {
		if err := p.validateWithPresetClaims(presetClaims, presetVerifier, presetCELRules); err != nil {
			p.metrics.validationErrors.WithLabels(issuerLabels).Inc()
			p.logger.ErrorContext(ctx, "failed to validate token", slog.Any("error", err))
			return nil, err
		}

		if err := p.finalizeValidatedToken(ctx, token, presetClaims, presetHeader); err != nil {
			p.metrics.validationErrors.WithLabels(issuerLabels).Inc()
			return nil, err
		}
		return presetClaims, nil
	}

	// Apply function options.
	// Fast path: when no per-call overrides, reuse pre-built options and CEL rules
	// to avoid cloneVerifierOptions + compileVerifierCELRules allocations.
	var ops *verifierOptions
	var compiledCELRules []celPreCompiledValidationRule
	if len(opt) == 0 && p.verifierOptions != nil {
		ops = p.verifierOptions
		compiledCELRules = p.celCompiledRules
	} else {
		ops = cloneVerifierOptions(p.verifierOptions)
		applyValidationOptions(ops, opt...)
		ops.buildIgnoredSet()
		compiledCELRules = compileVerifierCELRules(ops, p.logger)
	}

	claims, header, err := p.parseAndValidateToken(token, ops, compiledCELRules)
	if err != nil {
		p.metrics.validationErrors.WithLabels(issuerLabels).Inc()
		p.logger.ErrorContext(ctx, "failed to validate token", slog.Any("error", err))
		return nil, err
	}

	if err := p.finalizeValidatedToken(ctx, token, claims, header); err != nil {
		p.metrics.validationErrors.WithLabels(issuerLabels).Inc()
		return nil, err
	}

	return claims, nil
}

// checkTokenRevocation validates token revocation status using introspection or local storage.
//
// This is the pre-signature-verification check. Introspection always runs
// (the IdP is the source of truth). For local revocation storage it runs
// only when the storage is keyed on the full token — claims (jti) or header
// (kid) extracted from an unverified JWT are attacker-controlled and must
// not drive security-relevant lookups, so jti/kid checks are deferred to
// [Provider.checkTokenRevocationVerified] which runs after signature
// verification.
//
// Introspection errors are fatal by default: the token is rejected with
// [ErrIntrospection] when the endpoint is unreachable ([WithIntrospection]).
// Enable [WithIntrospectionFailOpen] to instead log the error and accept the
// token on its signature alone.
func (p *Provider) checkTokenRevocation(ctx context.Context, token string) error {
	if p.opts.introspectionEnabled {
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

	if p.revocationStorage == nil {
		return nil
	}

	// jti/kid lookups need verified claims/header — defer to
	// checkTokenRevocationVerified called after signature verification.
	if p.opts.revocationItemType == RevocationItemTypeJTI || p.opts.revocationItemType == RevocationItemTypeKID {
		return nil
	}

	isRevoked, err := p.revocationStorage.IsRevoked(ctx, token)
	if err == nil && isRevoked {
		p.logger.DebugContext(ctx, "token found in revocation storage",
			"item_type", p.opts.revocationItemType)
		return ErrTokenRevoked
	}

	return nil
}

// checkTokenRevocationVerified runs the revocation lookup that depends on
// signature-verified data (jti claim or kid header). It must only be called
// after [Provider.parseAndValidateToken] or [Provider.parseTokenWithoutClaimsValidation]
// has returned successfully — passing unverified claims/header would
// reintroduce the bypass this method is designed to prevent.
//
// For full-token revocation storage this is a no-op: the pre-verification
// check in [Provider.checkTokenRevocation] already covers that case.
func (p *Provider) checkTokenRevocationVerified(ctx context.Context, token string, claims jwt.MapClaims, header map[string]any) error {
	if p.revocationStorage == nil {
		return nil
	}
	if p.opts.revocationItemType != RevocationItemTypeJTI && p.opts.revocationItemType != RevocationItemTypeKID {
		return nil
	}

	item := revocationItemFromVerifiedClaims(p.opts.revocationItemType, claims, header, token)
	isRevoked, err := p.revocationStorage.IsRevoked(ctx, item)
	if err == nil && isRevoked {
		p.logger.DebugContext(ctx, "item found in revocation storage",
			"item_type", p.opts.revocationItemType,
			"item", item)
		return ErrTokenRevoked
	}

	return nil
}

// checkTokenRevocationVerifiedCached runs the post-verification revocation
// lookup against cached claims. The cache is keyed by the full token, so a
// cache hit proves the same token was previously signature-verified — making
// it safe to recover the JWT header via ParseUnverified solely to read the
// kid for kid-type revocation lookups.
func (p *Provider) checkTokenRevocationVerifiedCached(ctx context.Context, token string, claims map[string]any) error {
	if p.revocationStorage == nil {
		return nil
	}
	if p.opts.revocationItemType != RevocationItemTypeJTI && p.opts.revocationItemType != RevocationItemTypeKID {
		return nil
	}

	var header map[string]any
	if p.opts.revocationItemType == RevocationItemTypeKID {
		if t, _, err := new(jwt.Parser).ParseUnverified(token, jwt.MapClaims{}); err == nil {
			header = t.Header
		}
	}
	return p.checkTokenRevocationVerified(ctx, token, jwt.MapClaims(claims), header)
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

// finalizeValidatedToken centralizes the work that must run after every
// successful signature/claims verification path: the post-verification
// revocation lookup followed by caching the validated claims. Keeping it
// in one helper guarantees that any future validation path inherits the
// same security boundary without ad-hoc duplication.
func (p *Provider) finalizeValidatedToken(ctx context.Context, token string, claims jwt.MapClaims, header map[string]any) error {
	if err := p.checkTokenRevocationVerified(ctx, token, claims, header); err != nil {
		return err
	}
	p.cacheValidatedClaims(ctx, token, claims)
	return nil
}

func (p *Provider) getDiscoveryInfo(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.discoveryURL, nil)
	if err != nil {
		return err
	}

	resp, err := p.client.Do(req) //nolint:bodyclose
	if err != nil {
		return coreerrs.Wrapf(ErrDiscovery, "%s", err)
	}
	defer drainAndClose(resp)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return coreerrs.Wrapf(ErrDiscovery, "%s", err)
	}

	if resp.StatusCode != http.StatusOK {
		return coreerrs.Wrapf(ErrDiscovery, "unexpected response status: %s", resp.Status)
	}

	if ct := resp.Header.Get("Content-Type"); ct != "" && !strings.Contains(ct, "application/json") {
		return coreerrs.Wrapf(ErrDiscovery, "unexpected content type: %s", ct)
	}

	var dInfo discoveryInfo
	if err = json.Unmarshal(body, &dInfo); err != nil {
		return coreerrs.Wrapf(ErrDiscovery, "failed to unmarshal response body: %v", err)
	}

	if !dInfo.IsValid() {
		return coreerrs.Wrap(ErrDiscovery, "received uncompleted discovery info")
	}

	p.discoveryInfo = &dInfo

	return nil
}

// initializeJWKS sets up JWKS storage with automatic refresh.
func (p *Provider) initializeJWKS() error {
	// Configure JWKS storage options with caching and automatic refresh
	storageOptions := jwkset.HTTPClientStorageOptions{
		Client:      p.client,
		Ctx:         p.backgroundCtx,
		HTTPTimeout: p.opts.jwksHTTPTimeout,
		RefreshErrorHandler: func(ctx context.Context, err error) {
			// Log JWKS refresh errors and expose the staleness age so
			// operators tracking [WithJWKSMaxStaleness] can correlate the
			// log line with the staleness budget. The cached keys keep
			// being served until a refresh succeeds; whether validation
			// keeps trusting them is controlled by checkJWKSStaleness.
			p.logger.ErrorContext(ctx, "failed to refresh JWKS keys",
				slog.Any("error", err),
				slog.String("jwks_url", p.discoveryInfo.JwksURL),
				slog.Duration("since_last_refresh", p.jwksAge()))
		},
	}

	// Create JWKS storage from HTTP endpoint. Internal refresh interval is NOT set
	// to allow external management via RefreshJWKS and a scheduler.
	storage, err := jwkset.NewStorageFromHTTP(p.discoveryInfo.JwksURL, storageOptions)
	if err != nil {
		return coreerrs.WrapOperation(err, "initialize JWKS storage")
	}

	// Create keyfunc with the configured storage
	p.jwks, err = keyfunc.New(keyfunc.Options{
		Ctx:     p.backgroundCtx,
		Storage: storage,
	})
	if err != nil {
		return coreerrs.WrapOperation(err, "create JWKS keyfunc")
	}

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

// validateClaims performs custom claims validation based on verifier options.
func (p *Provider) validateClaims(claims map[string]any, ops *verifierOptions, compiledCELRules []celPreCompiledValidationRule) error {
	if ops.withoutClaimsValidation {
		return nil
	}

	if err := p.checkAudienceConfigured(ops); err != nil {
		return err
	}

	// Use the pre-built ignored claims set (populated by buildIgnoredSet).
	ignored := ops.ignoredClaimsSet

	if err := validateRequiredClaims(claims, ops, ignored); err != nil {
		return err
	}

	if err := validateExpectedClaims(claims, ops, ignored); err != nil {
		return err
	}

	if err := validateAllowedClientIDs(claims, ops.allowedClientIDs); err != nil {
		return err
	}

	if err := validateRequiredScopes(claims, ops.requiredScopes); err != nil {
		return err
	}

	if err := validateAuthorizedParty(claims, ops.requireAuthorizedParty, ops.allowedAuthorizedParties); err != nil {
		return err
	}

	if err := validateTokenLifetime(claims, ops.maxTokenLifetime); err != nil {
		return err
	}

	return validateCELRules(claims, compiledCELRules)
}

// checkAudienceConfigured enforces the provider's [AudienceFailureMode] when
// validation runs without an expected audience. With an expected audience set
// its value is enforced by the jwt parser ([Provider.buildParserOptions]); the
// `aud` claim being merely present (DefaultRequiredClaims) does not bind the
// token to this service, so an unconfigured audience is treated per the mode:
// enforce rejects, warn logs and continues, disabled is silent.
func (p *Provider) checkAudienceConfigured(ops *verifierOptions) error {
	if len(ops.audience) > 0 {
		return nil
	}

	switch p.opts.audienceFailureMode {
	case AudienceFailureModeWarn:
		p.logger.Warn("token validated without an expected audience; cross-audience replay is possible " +
			"(configure WithValidationAudience or set WithAudienceFailureMode)")
		return nil
	case AudienceFailureModeDisabled:
		return nil
	default: // AudienceFailureModeEnforce
		return coreerrs.Wrap(ErrAudienceNotConfigured,
			"refusing to validate token without an expected audience (set WithValidationAudience or relax via WithAudienceFailureMode)")
	}
}

func isIgnoredClaim(ignored map[string]struct{}, claim string) bool {
	if len(ignored) == 0 {
		return false
	}
	_, exists := ignored[claim]
	return exists
}

func validateRequiredClaims(claims map[string]any, ops *verifierOptions, ignored map[string]struct{}) error {
	required := ops.requiredClaims
	if len(required) == 0 {
		required = DefaultRequiredClaims
	}

	for _, claim := range required {
		if isIgnoredClaim(ignored, claim) {
			continue
		}

		if claim == "sub" && ops.allowMissingSubject {
			continue
		}

		value, exists := claims[claim]
		if !exists {
			return coreerrs.Wrapf(ErrInvalidToken, "missing required claim '%s'", claim)
		}

		if claim == "sub" {
			subStr, ok := value.(string)
			if !ok || subStr == "" {
				return coreerrs.Wrap(ErrInvalidToken, "claim 'sub' cannot be empty")
			}
		}
	}

	return nil
}

func validateExpectedClaims(claims map[string]any, ops *verifierOptions, ignored map[string]struct{}) error {
	for claim, expectedValue := range ops.expectedClaims {
		if isIgnoredClaim(ignored, claim) {
			continue
		}

		actualValue, exists := claims[claim]
		if !exists {
			return coreerrs.Wrapf(ErrInvalidToken, "missing expected claim '%s'", claim)
		}

		actualStr := fmt.Sprint(actualValue)
		if actualStr != expectedValue {
			return coreerrs.Wrapf(ErrInvalidToken, "claim '%s' has unexpected value", claim)
		}
	}

	return nil
}

func validateAllowedClientIDs(claims map[string]any, allowed []string) error {
	if len(allowed) == 0 {
		return nil
	}

	clientID, exists := claims["client_id"]
	if !exists {
		clientID, exists = claims["azp"]
	}
	if !exists {
		return coreerrs.Wrap(ErrInvalidToken, "missing 'client_id' or 'azp' claim for client ID validation")
	}

	clientIDStr, ok := clientID.(string)
	if !ok {
		return coreerrs.Wrap(ErrInvalidToken, "claim 'client_id' is not a string")
	}

	if !slices.Contains(allowed, clientIDStr) {
		return coreerrs.Wrap(ErrInvalidToken, "client_id is not in the allowed list")
	}

	return nil
}

func validateRequiredScopes(claims map[string]any, requiredScopes []string) error {
	if len(requiredScopes) == 0 {
		return nil
	}

	tokenScopes, err := strictScopeClaim(claims)
	if err != nil {
		return err
	}

	for _, requiredScope := range requiredScopes {
		if slices.Contains(tokenScopes, requiredScope) {
			return nil
		}
	}

	return coreerrs.Wrap(ErrInvalidToken, "token does not contain any of the required scopes")
}

func strictScopeClaim(claims map[string]any) ([]string, error) {
	scopeValue, exists := claims["scope"]
	if !exists {
		return nil, coreerrs.Wrap(ErrInvalidToken, "missing 'scope' claim for scope validation")
	}

	scopes, ok := normalizeScopeValue(scopeValue)
	if !ok {
		return nil, coreerrs.Wrap(ErrInvalidToken, "claim 'scope' has invalid type")
	}

	return scopes, nil
}

func validateAuthorizedParty(claims map[string]any, require bool, allowedParties []string) error {
	if !require {
		return nil
	}

	azp, exists := claims["azp"]
	if !exists {
		return coreerrs.Wrap(ErrInvalidToken, "missing required 'azp' (authorized party) claim")
	}

	azpStr, ok := azp.(string)
	if !ok {
		return coreerrs.Wrap(ErrInvalidToken, "claim 'azp' is not a string")
	}

	if len(allowedParties) > 0 && !slices.Contains(allowedParties, azpStr) {
		return coreerrs.Wrap(ErrInvalidToken, "authorized party is not in the allowed list")
	}

	return nil
}

func validateTokenLifetime(claims map[string]any, maxLifetime time.Duration) error {
	if maxLifetime <= 0 {
		return nil
	}

	expTime, expOk := parseClaimAsInt64(claims, "exp")
	iatTime, iatOk := parseClaimAsInt64(claims, "iat")

	if !expOk || !iatOk {
		return coreerrs.Wrap(ErrInvalidToken, "missing or invalid 'exp' or 'iat' claim for lifetime validation")
	}

	lifetime := time.Duration(expTime-iatTime) * time.Second
	if lifetime < 0 {
		return coreerrs.Wrap(ErrInvalidToken, "invalid token lifetime (exp < iat)")
	}

	if lifetime > maxLifetime {
		return coreerrs.Wrapf(ErrInvalidToken, "token lifetime %v exceeds maximum allowed %v", lifetime, maxLifetime)
	}

	return nil
}

func validateCELRules(claims map[string]any, compiled []celPreCompiledValidationRule) error {
	for i := range compiled {
		rule := &compiled[i]
		if rule.matcher(claims) {
			continue
		}

		// Use custom message if provided, otherwise use default format
		if rule.message != "" {
			return coreerrs.Wrapf(ErrCELValidation, "%s", rule.message)
		}
		return coreerrs.Wrapf(ErrCELValidation, "rule '%s' failed (expression: %s)",
			rule.name, strings.ReplaceAll(rule.expression, "\n", " "))
	}

	return nil
}

// parseClaimAsInt64 extracts a numeric claim as int64, returning false if missing or invalid.
func parseClaimAsInt64(claims map[string]any, claimName string) (int64, bool) {
	value, ok := claims[claimName]
	if !ok {
		return 0, false
	}

	switch v := value.(type) {
	case float64:
		return int64(v), true
	case int64:
		return v, true
	case int:
		return int64(v), true
	default:
		return 0, false
	}
}

// getTokenExpirationTTL returns time until token expiration, or 0 if expired/invalid.
func getTokenExpirationTTL(claims map[string]any) time.Duration {
	expTime, ok := parseClaimAsInt64(claims, "exp")
	if !ok {
		return 0
	}

	now := time.Now().Unix()
	ttl := expTime - now
	if ttl <= 0 {
		return 0
	}

	return time.Duration(ttl) * time.Second
}

// buildParserOptions converts verifier options to JWT parser options.
// Used for both jwt.Parse() and jwt.NewValidator().
func (p *Provider) buildParserOptions(ops *verifierOptions) []jwt.ParserOption {
	if ops == nil {
		return nil
	}

	// Early return if all validation is disabled
	if ops.withoutClaimsValidation {
		return []jwt.ParserOption{jwt.WithoutClaimsValidation()}
	}

	var parserOpts []jwt.ParserOption

	// Restrict allowed signing algorithms to prevent algorithm confusion attacks.
	// Use configured methods if set, otherwise fall back to DefaultValidMethods.
	validMethods := ops.validMethods
	if len(validMethods) == 0 {
		validMethods = DefaultValidMethods
	}
	parserOpts = append(parserOpts, jwt.WithValidMethods(validMethods))

	// Add leeway (clock skew tolerance)
	if ops.leeway > 0 {
		parserOpts = append(parserOpts, jwt.WithLeeway(ops.leeway))
	}

	// Verify issued-at claim if requested
	if ops.issuedAt {
		parserOpts = append(parserOpts, jwt.WithIssuedAt())
	}

	// Require the exp claim to be present when explicitly opted in. The
	// jwt-go library always validates exp when it appears in the token;
	// WithExpirationRequired adds the stricter contract that a token
	// without exp is rejected (matches `verify_expiration: true` in the
	// service config).
	if ops.expirationRequired {
		parserOpts = append(parserOpts, jwt.WithExpirationRequired())
	}

	// Same contract for the nbf claim — `verify_not_before: true` in the
	// service config translates to "nbf must be present and respected".
	if ops.notBeforeRequired {
		parserOpts = append(parserOpts, jwt.WithNotBeforeRequired())
	}

	// Always verify issuer (use discovery issuer as fallback)
	issuer := ops.issuer
	if issuer == "" {
		issuer = p.discoveryInfo.Issuer
	}
	parserOpts = append(parserOpts, jwt.WithIssuer(issuer))

	// Verify subject if specified
	if ops.subject != "" {
		parserOpts = append(parserOpts, jwt.WithSubject(ops.subject))
	}

	// Verify audience if specified
	if len(ops.audience) > 0 {
		parserOpts = append(parserOpts, jwt.WithAudience(ops.audience...))
	}

	return parserOpts
}

// parseAndValidateToken parses and validates a JWT with the given options.
// Returns the verified claims and token header.
func (p *Provider) parseAndValidateToken(
	token string,
	ops *verifierOptions,
	compiledCELRules []celPreCompiledValidationRule,
) (jwt.MapClaims, map[string]any, error) {
	parserOpts := p.buildParserOptions(ops)

	jwtToken, err := jwt.Parse(token, p.jwks.Keyfunc, parserOpts...)
	if err != nil {
		return nil, nil, coreerrs.Wrapf(ErrInvalidToken, "%s", err)
	}

	if !jwtToken.Valid {
		return nil, nil, coreerrs.Wrap(ErrInvalidToken, "token is invalid")
	}

	claims, ok := jwtToken.Claims.(jwt.MapClaims)
	if !ok {
		return nil, nil, coreerrs.Wrap(ErrInvalidToken, "invalid claims type")
	}

	if err := p.validateClaims(claims, ops, compiledCELRules); err != nil {
		return nil, nil, err
	}

	return claims, jwtToken.Header, nil
}

func (p *Provider) validateWithPresetClaims(claims map[string]any, ops *verifierOptions, compiledCELRules []celPreCompiledValidationRule) error {
	if ops == nil || ops.withoutClaimsValidation {
		return nil
	}

	validatorOpts := p.buildParserOptions(ops)
	validator := jwt.NewValidator(validatorOpts...)

	if err := validator.Validate(jwt.MapClaims(claims)); err != nil {
		return coreerrs.Wrapf(ErrInvalidToken, "%s", err)
	}

	return p.validateClaims(claims, ops, compiledCELRules)
}

// cacheValidatedClaims saves claims to cache with TTL based on token expiration.
func (p *Provider) cacheValidatedClaims(ctx context.Context, token string, claims map[string]any) {
	if p.tokenCache == nil {
		return
	}

	cacheKey := tokenCacheKey(p.opts.tokensCacheKeyPrefix, token)
	ttl := getTokenExpirationTTL(claims)
	_ = p.tokenCache.Save(ctx, cacheKey, claims, ttl) //nolint:errcheck
}

func cloneVerifierOptions(base *verifierOptions) *verifierOptions {
	if base == nil {
		return &verifierOptions{}
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
