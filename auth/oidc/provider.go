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

	corectx "github.com/altessa-s/go-atlas/core/context"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

var (
	ErrInvalidToken     = errors.New("invalid token")
	ErrCELValidation    = errors.New("CEL validation failed")
	ErrDiscovery        = errors.New("discovery failed")
	ErrIntrospection    = errors.New("introspection failed")
	ErrTokenRevoked     = errors.New("token has been revoked")
	ErrSchedulerManaged = errors.New("function is managed by scheduler, direct calls not allowed")
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

	// Create Provider from options
	p := &Provider{
		opts:              o,
		discoveryURL:      discoveryURL,
		client:            o.client,
		logger:            o.logger,
		tokenCache:        o.tokenCache,
		revocationStorage: o.revocationStorage,
		revocationFilter:  o.revocationFilter,
		revocationLoader:  o.revocationLoader,
		verifierOptions:   o.verifierOptions,
		scheduler:         o.scheduler,
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

// parseToken parses a JWT and verifies its signature, returning the claims.
// Valid signing methods are always enforced (DefaultValidMethods) to prevent
// algorithm confusion attacks. Optional parser options can customize other behavior.
func (p *Provider) parseToken(token string, opts ...jwt.ParserOption) (map[string]any, error) {
	// Prepend valid methods restriction so it applies to all parse paths.
	// Callers can override by passing their own jwt.WithValidMethods (last wins).
	opts = append([]jwt.ParserOption{jwt.WithValidMethods(DefaultValidMethods)}, opts...)

	jwtToken, err := jwt.Parse(token, p.jwks.Keyfunc, opts...)
	if err != nil {
		return nil, err
	}

	if !jwtToken.Valid {
		return nil, coreerrs.Wrap(ErrInvalidToken, "token is invalid")
	}

	claims, ok := jwtToken.Claims.(jwt.MapClaims)
	if !ok {
		return nil, coreerrs.Wrap(ErrInvalidToken, "invalid claims type")
	}

	return claims, nil
}

// verifySignature verifies JWT signature and returns parsed claims.
func (p *Provider) verifySignature(token string) (map[string]any, error) {
	return p.parseToken(token)
}

// parseTokenWithoutClaimsValidation parses a JWT without validating claims.
func (p *Provider) parseTokenWithoutClaimsValidation(token string) (map[string]any, error) {
	return p.parseToken(token, jwt.WithoutClaimsValidation())
}

// ValidateToken validates a JWT and returns its claims using default options.
func (p *Provider) ValidateToken(ctx context.Context, token string) (map[string]any, error) {
	return p.ValidateTokenWithOptions(ctx, token)
}

// ValidateTokenWithOptions validates a JWT with custom validation options.
func (p *Provider) ValidateTokenWithOptions(ctx context.Context, token string, opt ...ValidationOption) (map[string]any, error) {
	// Reject empty tokens immediately
	if token == "" {
		return nil, coreerrs.Wrap(ErrInvalidToken, "token is empty")
	}

	if err := p.checkTokenRevocation(ctx, token); err != nil {
		return nil, err
	}

	// Try to get cached claims if caching is enabled (only after revocation check)
	if p.tokenCache != nil {
		cacheKey := tokenCacheKey(p.opts.tokensCacheKeyPrefix, token)
		var claims map[string]any
		if err := p.tokenCache.Get(ctx, cacheKey, &claims); err == nil {
			// Cache hit - return cached claims
			return claims, nil
		}
		// Cache miss or error - fall through to validation
	}

	var presetClaims map[string]any
	var presetVerifier *verifierOptions
	var presetCELRules []celPreCompiledValidationRule

	// Fast path: no presets, no default options, and no overrides
	if len(opt) == 0 && p.verifierOptions == nil && len(p.opts.presetRules) == 0 {
		claims, err := p.parseAndValidateToken(token, &verifierOptions{}, nil)
		if err != nil {
			return nil, err
		}
		p.cacheValidatedClaims(ctx, token, claims)
		return claims, nil
	}

	// If no options provided and preset selection rules are configured,
	// try automatic preset selection
	if len(opt) == 0 && len(p.opts.presetRules) > 0 {
		// Step 1: Verify signature FIRST for security (without claim validation)
		claimsForPreset, err := p.parseTokenWithoutClaimsValidation(token)
		if err != nil {
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
			p.logger.ErrorContext(ctx, "failed to validate token", slog.Any("error", err))
			return nil, err
		}

		p.cacheValidatedClaims(ctx, token, presetClaims)
		return presetClaims, nil
	}

	// Apply function options
	ops := cloneVerifierOptions(p.verifierOptions)
	applyValidationOptions(ops, opt...)
	compiledCELRules := compileVerifierCELRules(ops, p.logger)

	claims, err := p.parseAndValidateToken(token, ops, compiledCELRules)
	if err != nil {
		p.logger.ErrorContext(ctx, "failed to validate token", slog.Any("error", err))
		return nil, err
	}

	p.cacheValidatedClaims(ctx, token, claims)

	return claims, nil
}

// checkTokenRevocation validates token revocation status using introspection or local storage.
// Introspection errors are logged but treated as non-fatal to avoid auth outages.
func (p *Provider) checkTokenRevocation(ctx context.Context, token string) error {
	if p.opts.introspectionEnabled {
		introspectionResp, err := p.IntrospectToken(ctx, token)
		if err != nil {
			p.logger.WarnContext(ctx, "token introspection failed, continuing with signature validation", slog.Any("error", err))
			return nil
		}
		if !introspectionResp.Active {
			return ErrTokenRevoked
		}
		return nil
	}

	if p.revocationStorage == nil {
		return nil
	}

	item := p.revocationItemFromToken(token)
	isRevoked, err := p.revocationStorage.IsRevoked(ctx, item)
	if err == nil && isRevoked {
		p.logger.DebugContext(ctx, "item found in revocation storage",
			"item_type", p.opts.revocationItemType,
			"item", item)
		return ErrTokenRevoked
	}

	return nil
}

func (p *Provider) revocationItemFromToken(token string) string {
	item := token
	if p.opts.revocationItemType != RevocationItemTypeJTI && p.opts.revocationItemType != RevocationItemTypeKID {
		return item
	}

	if t, _, err := new(jwt.Parser).ParseUnverified(token, jwt.MapClaims{}); err == nil {
		switch p.opts.revocationItemType {
		case RevocationItemTypeJTI:
			if claims, ok := t.Claims.(jwt.MapClaims); ok {
				if jti, ok := claims[RevocationItemTypeJTI].(string); ok {
					item = jti
				}
			}
		case RevocationItemTypeKID:
			if kid, ok := t.Header[RevocationItemTypeKID].(string); ok {
				item = kid
			}
		}
	}

	return item
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
			// Log JWKS refresh errors but don't fail
			// The cached keys will continue to be used until refresh succeeds
			p.logger.ErrorContext(ctx, "failed to refresh JWKS keys",
				slog.Any("error", err),
				slog.String("jwks_url", p.discoveryInfo.JwksURL))
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

	return nil
}

// validateClaims performs custom claims validation based on verifier options.
func (p *Provider) validateClaims(claims map[string]any, ops *verifierOptions, compiledCELRules []celPreCompiledValidationRule) error {
	if ops.withoutClaimsValidation {
		return nil
	}

	ignored := buildIgnoredClaimsSet(ops.ignoredClaims)

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

func buildIgnoredClaimsSet(ignoredClaims []string) map[string]struct{} {
	if len(ignoredClaims) == 0 {
		return nil
	}

	set := make(map[string]struct{}, len(ignoredClaims))
	for _, claim := range ignoredClaims {
		set[claim] = struct{}{}
	}

	return set
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
func (p *Provider) parseAndValidateToken(token string, ops *verifierOptions, compiledCELRules []celPreCompiledValidationRule) (jwt.MapClaims, error) {
	parserOpts := p.buildParserOptions(ops)

	jwtToken, err := jwt.Parse(token, p.jwks.Keyfunc, parserOpts...)
	if err != nil {
		return nil, coreerrs.Wrapf(ErrInvalidToken, "%s", err)
	}

	if !jwtToken.Valid {
		return nil, coreerrs.Wrap(ErrInvalidToken, "token is invalid")
	}

	claims, ok := jwtToken.Claims.(jwt.MapClaims)
	if !ok {
		return nil, coreerrs.Wrap(ErrInvalidToken, "invalid claims type")
	}

	if err := p.validateClaims(claims, ops, compiledCELRules); err != nil {
		return nil, err
	}

	return claims, nil
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

	return &cloned
}

func applyValidationOptions(ops *verifierOptions, options ...ValidationOption) {
	for _, opt := range options {
		opt(ops)
	}
}
