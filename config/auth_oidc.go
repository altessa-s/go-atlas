// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"regexp"
	"time"

	"github.com/go-ozzo/ozzo-validation/v4/is"

	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for OIDC configuration.
const (
	defaultOIDCClockSkew              = 10 * time.Second
	defaultOIDCCacheTokensKeyPrefix   = "tokens:"
	defaultOIDCCacheRevokedKeyPrefix  = "revoked-tokens:"
	defaultOIDCJwksRefreshEnabled     = true
	defaultOIDCJwksRefreshSchedule    = "0 0 * * * *"
	defaultOIDCJwksHTTPTimeout        = 30 * time.Second
	defaultOIDCRevocationEnabled      = false
	defaultOIDCRevocationSyncEnabled  = true
	defaultOIDCRevocationSyncSchedule = "0 0 * * * *"
	defaultOIDCRevocationItemType     = "token"
	defaultOIDCCacheEnabled           = false
)

var (
	// namePattern validates preset names (lowercase alphanumeric with hyphens, starting with a letter).
	namePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
)

// OIDC represents the configuration for OpenID Connect authentication.
// It contains all necessary parameters for OIDC provider integration including
// token validation, client credentials, and OIDCJwks management.
type OIDC struct {
	// DiscoveryUrl is the URL of the OpenID Connect discovery endpoint
	DiscoveryUrl string `yaml:"discoveryUrl"`

	// ClockSkew is the acceptable time difference when validating timestamps
	ClockSkew time.Duration `yaml:"clockSkew" default:"10s"`

	// Cache contains optional token caching settings
	Cache *OIDCCache `yaml:"cache" default:"-"`

	// Validation contains optional token validation settings
	Validation *OIDCValidation `yaml:"validation" default:"-"`

	// ClientCredentials holds shared OAuth2 client credentials for server-to-server operations
	ClientCredentials *OIDCClientCredentials `yaml:"clientCredentials" default:"-"`

	// JWKS contains optional JWKS-specific settings
	JWKS *OIDCJwks `yaml:"jwks" default:"-"`

	// Introspection controls RFC 7662 token introspection (credentials come from ClientCredentials)
	Introspection *OIDCIntrospection `yaml:"introspection" default:"-"`

	// Presets contains optional validation presets configuration
	Presets *OIDCPresets `yaml:"presets" default:"-"`

	// Revocation contains optional token/key revocation settings
	Revocation *OIDCRevocation `yaml:"revocation" default:"-"`

	// Proxy configures the outbound HTTP proxy for every OIDC call:
	// discovery, JWKS refresh, introspection, userinfo, and URL-based
	// revocation loaders. When omitted entirely, the standard
	// HTTP_PROXY/HTTPS_PROXY/NO_PROXY environment variables apply.
	Proxy *HTTPProxy `yaml:"proxy" default:"-"`
}

// DefaultOIDC returns an OIDC configuration with default values.
// Note: DiscoveryUrl is left as zero value since it is a required field.
func DefaultOIDC() OIDC {
	return OIDC{
		ClockSkew: defaultOIDCClockSkew,
	}
}

// Validate performs validation on the OIDC configuration.
// It validates the discovery URL, clock skew, and nested configuration structures.
//
// Returns an error if any validation rules fail.
func (a *OIDC) Validate() error {
	return ValidateStruct(a,
		validation.Field(&a.DiscoveryUrl, validation.Required, is.URL),
		validation.Field(&a.ClockSkew, validation.Min(time.Second), validation.Max(time.Minute)),
		validation.Field(&a.Cache, validation.NilOrNotEmpty),
		validation.Field(&a.Validation, validation.NilOrNotEmpty),
		validation.Field(&a.ClientCredentials, validation.NilOrNotEmpty),
		validation.Field(&a.JWKS, validation.NilOrNotEmpty),
		validation.Field(&a.Introspection, validation.NilOrNotEmpty),
		validation.Field(&a.Introspection, validation.When(
			a.Introspection != nil && a.Introspection.Enabled,
			validation.By(func(_ any) error {
				if a.ClientCredentials == nil {
					return validation.NewError("validation_introspection_requires_credentials",
						"clientCredentials must be configured when introspection is enabled")
				}
				return nil
			}),
		)),
		validation.Field(&a.Presets, validation.NilOrNotEmpty),
		validation.Field(&a.Revocation, validation.NilOrNotEmpty),
		validation.Field(&a.Proxy),
	)
}

// IsCacheConfigured returns true if cache settings are configured.
func (a *OIDC) IsCacheConfigured() bool {
	return a.Cache != nil
}

// IsClientCredentialsConfigured returns true if client credentials are configured.
func (a *OIDC) IsClientCredentialsConfigured() bool {
	return a.ClientCredentials != nil
}

// IsIntrospectionEnabled returns true if introspection is enabled and client credentials are set.
func (a *OIDC) IsIntrospectionEnabled() bool {
	return a.Introspection != nil && a.Introspection.Enabled && a.ClientCredentials != nil
}

// IsValidationConfigured returns true if validation settings are configured.
func (a *OIDC) IsValidationConfigured() bool {
	return a.Validation != nil
}

// IsJWKSConfigured returns true if OIDCJwks settings are configured.
func (a *OIDC) IsJWKSConfigured() bool {
	return a.JWKS != nil
}

// IsPresetsConfigured returns true if presets settings are configured.
func (a *OIDC) IsPresetsConfigured() bool {
	return a.Presets != nil
}

// IsRevocationConfigured returns true if revocation settings are configured.
func (a *OIDC) IsRevocationConfigured() bool {
	return a.Revocation != nil
}

// OIDCCache represents token caching configuration.
// It controls caching behavior for validated and revoked tokens.
type OIDCCache struct {
	// Enabled enables caching of token validation results
	Enabled bool `yaml:"enabled" default:"false"`

	// TokensKeyPrefix is the prefix for validated token cache keys
	TokensKeyPrefix string `yaml:"tokensKeyPrefix" default:"tokens:"`

	// RevokedTokensKeyPrefix is the prefix for revoked token cache keys
	RevokedTokensKeyPrefix string `yaml:"revokedTokensKeyPrefix" default:"revoked-tokens:"`
}

// DefaultOIDCCache returns an OIDCCache configuration with default values.
func DefaultOIDCCache() OIDCCache {
	return OIDCCache{
		Enabled:                defaultOIDCCacheEnabled,
		TokensKeyPrefix:        defaultOIDCCacheTokensKeyPrefix,
		RevokedTokensKeyPrefix: defaultOIDCCacheRevokedKeyPrefix,
	}
}

// Validate performs validation on the OIDCCache configuration.
// It validates that cache key prefixes are not empty when caching is enabled.
//
// Returns an error if any validation rules fail.
func (c *OIDCCache) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.TokensKeyPrefix, validation.When(c.Enabled, validation.Required)),
		validation.Field(&c.RevokedTokensKeyPrefix, validation.When(c.Enabled, validation.Required)),
	)
}

// OIDCExpression represents a CEL expression for custom token validation.
// It allows defining custom validation logic with optional name and error message.
type OIDCExpression struct {
	// Expression is the CEL expression to evaluate (required)
	Expression string `yaml:"expression"`

	// Name is the rule identifier used in error messages (optional)
	Name string `yaml:"name"`
}

// Validate performs validation on the OIDCExpression configuration.
// It validates that the expression is not empty.
//
// Returns an error if any validation rules fail.
func (e *OIDCExpression) Validate() error {
	return ValidateStruct(e,
		validation.Field(&e.Expression, validation.Required),
	)
}

// OIDCValidation represents token validation settings for OIDC.
// It contains rules for validating JWT tokens including issuer, claims, and lifetime constraints.
type OIDCValidation struct {
	// Issuer is the expected issuer claim (iss) in the token
	Issuer *string `yaml:"issuer"`

	// Claims contains optional requirements for JWT token claims validation
	Claims *OIDCClaims `yaml:"claims" default:"-"`

	// MaxTokenLifetime is the maximum acceptable token lifetime
	MaxTokenLifetime time.Duration `yaml:"maxTokenLifetime"`

	// Expression is a CEL expression for custom token validation logic
	Expression *OIDCExpression `yaml:"expression" default:"-"`
}

// Validate performs validation on the OIDCValidation configuration.
// It validates the issuer URL format, claims configuration, and token lifetime constraints.
//
// Returns an error if any validation rules fail.
func (v *OIDCValidation) Validate() error {
	return ValidateStruct(v,
		validation.Field(&v.Issuer, validation.NilOrNotEmpty, validation.When(v.Issuer != nil, is.URL)),
		validation.Field(&v.Claims, validation.NilOrNotEmpty),
		validation.Field(&v.MaxTokenLifetime, validation.Min(time.Duration(0))),
	)
}

// IsClaimsConfigured returns true if claims validation is configured.
func (v *OIDCValidation) IsClaimsConfigured() bool {
	return v.Claims != nil
}

// OIDCClaims represents JWT token claims validation configuration.
// It defines requirements for validating claims in OIDC tokens including
// required claims, expected values, audience, scopes, and authorized parties.
type OIDCClaims struct {
	// Required is the list of claims that must be present in the token
	Required []string `yaml:"required"`

	// Expected defines exact values that specific claims must have
	Expected map[string]string `yaml:"expected"`

	// Ignored is the list of claims to skip during validation
	Ignored []string `yaml:"ignored"`

	// Audience is the expected audience claim in the token
	Audience []string `yaml:"audience"`

	// AllowedClientIds is the whitelist of client_id values for service-to-service calls
	AllowedClientIds []string `yaml:"allowedClientIds"`

	// RequiredScopes is the list of required scopes for service tokens
	RequiredScopes []string `yaml:"requiredScopes"`

	// RequireAuthorizedParty requires the 'azp' (authorized party) claim
	RequireAuthorizedParty bool `yaml:"requireAuthorizedParty"`

	// AllowedAuthorizedParties is the whitelist of allowed 'azp' claim values
	AllowedAuthorizedParties []string `yaml:"allowedAuthorizedParties"`

	// AllowMissingSubject permits tokens without 'sub' claim
	AllowMissingSubject bool `yaml:"allowMissingSubject"`
}

// Validate performs validation on the OIDCClaims configuration.
// It validates that required fields are not empty and that authorized parties
// are provided when RequireAuthorizedParty is enabled.
//
// Returns an error if any validation rules fail.
func (c *OIDCClaims) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.Required, validation.Each(validation.Required)),
		validation.Field(&c.Audience, validation.Each(validation.Required)),
		validation.Field(&c.AllowedClientIds, validation.Each(validation.Required)),
		validation.Field(&c.RequiredScopes, validation.Each(validation.Required)),
		validation.Field(&c.Ignored, validation.Each(validation.Required)),
		validation.Field(&c.Expected, validation.Each(validation.Required)),
		validation.Field(&c.AllowedAuthorizedParties,
			validation.When(c.RequireAuthorizedParty, validation.Required, validation.Each(validation.Required))),
	)
}

// OIDCClientCredentials holds OAuth2 client credentials shared across
// introspection and other server-to-server flows.
type OIDCClientCredentials struct {
	// ClientId is the OAuth2 client identifier
	ClientId string `yaml:"clientId"`

	// ClientSecret is the OAuth2 client secret
	ClientSecret Secret `yaml:"clientSecret"`
}

// Validate performs validation on the OIDCClientCredentials configuration.
// It validates that required credentials are present.
//
// Returns an error if any validation rules fail.
func (cc *OIDCClientCredentials) Validate() error {
	return ValidateStruct(cc,
		validation.Field(&cc.ClientId, validation.Required),
		validation.Field(&cc.ClientSecret, validation.Required),
	)
}

// OIDCJwks represents OIDCJwks-specific configuration.
// It controls the refresh behavior for JSON Web Key Sets used in token signature verification.
type OIDCJwks struct {
	// RefreshEnabled enables proactive JWKS refresh via an external scheduler.
	RefreshEnabled bool `yaml:"refreshEnabled" default:"true"`

	// RefreshSchedule is the cron expression for periodic JWKS refresh.
	RefreshSchedule string `yaml:"refreshSchedule" default:"0 0 * * * *"`

	// HTTPTimeout is the timeout for JWKS HTTP requests
	HTTPTimeout time.Duration `yaml:"httpTimeout" default:"30s"`
}

// DefaultOIDCJwks returns an OIDCJwks configuration with default values.
func DefaultOIDCJwks() OIDCJwks {
	return OIDCJwks{
		RefreshEnabled:  defaultOIDCJwksRefreshEnabled,
		RefreshSchedule: defaultOIDCJwksRefreshSchedule,
		HTTPTimeout:     defaultOIDCJwksHTTPTimeout,
	}
}

// Validate performs validation on the OIDCJwks configuration.
// It validates that the HTTP timeout is non-negative.
//
// Returns an error if any validation rules fail.
func (j *OIDCJwks) Validate() error {
	return ValidateStruct(j,
		validation.Field(&j.RefreshSchedule, validation.When(j.RefreshEnabled, validation.Required)),
		validation.Field(&j.HTTPTimeout, validation.Min(time.Duration(0))),
	)
}

// OIDCIntrospection controls RFC 7662 token introspection.
// Credentials come from the shared ClientCredentials section.
type OIDCIntrospection struct {
	// Enabled toggles introspection on or off
	Enabled bool `yaml:"enabled" default:"false"`

	// Strict makes ValidateToken reject the token whenever the introspection
	// endpoint cannot confirm it is active (network error, non-2xx response,
	// parse failure). Without strict mode an unreachable IdP silently
	// bypasses revocation: the token is accepted on its signature alone.
	// Recommended for production.
	Strict bool `yaml:"strict" default:"false"`
}

// OIDCPresets represents validation presets configuration.
// It allows defining multiple validation rule sets that can be dynamically selected
// based on token claims or custom logic.
type OIDCPresets struct {
	// List is the list of named validation presets
	List []OIDCPreset `yaml:"list"`

	// Selectors is the list of rules for automatic preset selection
	Selectors []OIDCSelector `yaml:"selectors"`
}

// Validate performs validation on the OIDCPresets configuration.
// It validates the list of presets and selectors, ensuring they are used together.
// Both presets.list and presets.selectors must be configured together or both must be empty.
//
// Returns an error if any validation rules fail.
func (p *OIDCPresets) Validate() error {
	hasPresets := len(p.List) > 0
	hasSelectors := len(p.Selectors) > 0

	// Check that both list and selectors are either both present or both empty (XOR)
	if hasPresets != hasSelectors {
		return validation.Errors{
			"presets": validation.NewError("validation_presets_selectors_required", "presets.list and presets.selectors must be configured together"),
		}.Filter()
	}

	return ValidateStruct(p,
		validation.Field(&p.List, validation.Each(validation.Required)),
		validation.Field(&p.Selectors, validation.Each(validation.Required)),
	)
}

// OIDCPreset represents a named validation preset.
// Each preset defines a complete set of validation rules that can be applied to tokens.
type OIDCPreset struct {
	// Name is the unique preset identifier
	Name string `yaml:"name"`

	// Issuer is the expected issuer claim (iss) for this preset
	Issuer *string `yaml:"issuer"`

	// MaxTokenLifetime is the maximum acceptable token lifetime for this preset
	MaxTokenLifetime time.Duration `yaml:"maxTokenLifetime"`

	// Expression is a CEL expression for custom token validation logic
	Expression *OIDCExpression `yaml:"expression" default:"-"`

	// Claims contains optional requirements for JWT token claims validation
	Claims *OIDCClaims `yaml:"claims" default:"-"`
}

// Validate performs validation on the OIDCPreset configuration.
// It validates the preset name, issuer, and claims configuration.
//
// Returns an error if any validation rules fail.
func (p *OIDCPreset) Validate() error {
	return ValidateStruct(p,
		validation.Field(&p.Name, validation.Required, validation.Match(namePattern)),
		validation.Field(&p.Issuer, validation.NilOrNotEmpty, validation.When(p.Issuer != nil, is.URL)),
		validation.Field(&p.MaxTokenLifetime, validation.Min(time.Duration(0))),
		validation.Field(&p.Claims, validation.NilOrNotEmpty),
	)
}

// IsClaimsConfigured returns true if claims validation is configured.
func (p *OIDCPreset) IsClaimsConfigured() bool {
	return p.Claims != nil
}

// OIDCSelector represents a rule for automatic preset selection.
// Selectors are evaluated in priority order to dynamically choose validation presets
// based on token claims.
type OIDCSelector struct {
	// Expression is the CEL expression that evaluates to true/false to select this preset
	Expression string `yaml:"expression"`

	// Priority determines the evaluation order (higher values = higher priority)
	Priority int `yaml:"priority"`

	// PresetName is the name of the preset to apply when this selector matches
	PresetName string `yaml:"presetName"`
}

// Validate performs validation on the OIDCSelector configuration.
// It validates the CEL expression, priority, and preset name.
//
// Returns an error if any validation rules fail.
func (s *OIDCSelector) Validate() error {
	return ValidateStruct(s,
		validation.Field(&s.Expression, validation.Required),
		validation.Field(&s.PresetName, validation.Required),
	)
}

// OIDCRevocation represents token or key revocation configuration.
type OIDCRevocation struct {
	// Enabled enables revocation checking.
	Enabled bool `yaml:"enabled" default:"false"`

	// SyncEnabled enables periodic synchronization of the revocation storage
	// via an external scheduler.
	SyncEnabled bool `yaml:"syncEnabled" default:"true"`

	// SyncSchedule is the cron expression for periodic revocation synchronization.
	SyncSchedule string `yaml:"syncSchedule" default:"0 0 * * * *"`

	// ItemType specifies what items are being revoked: "token", "jti", or "kid"
	ItemType string `yaml:"itemType" default:"token"`

	// Filter contains configuration for the probabilistic filter used for revocation
	Filter *ProbabilisticFilterConfig `yaml:"filter"`

	// Source contains configuration for fetching the revocation list
	Source *OIDCRevocationSource `yaml:"source"`
}

// DefaultOIDCRevocation returns an OIDCRevocation configuration with default values.
func DefaultOIDCRevocation() OIDCRevocation {
	return OIDCRevocation{
		Enabled:      defaultOIDCRevocationEnabled,
		SyncEnabled:  defaultOIDCRevocationSyncEnabled,
		SyncSchedule: defaultOIDCRevocationSyncSchedule,
		ItemType:     defaultOIDCRevocationItemType,
	}
}

// Validate performs validation on the OIDCRevocation configuration.
func (r *OIDCRevocation) Validate() error {
	return ValidateStruct(r,
		validation.Field(&r.SyncSchedule, validation.When(r.SyncEnabled && r.Enabled, validation.Required)),
		validation.Field(&r.ItemType, validation.In("token", "jti", "kid")),
		validation.Field(&r.Filter, validation.Required),
		validation.Field(&r.Source, validation.Required),
	)
}

// OIDCRevocationSource represents the source for a revocation list.
type OIDCRevocationSource struct {
	// URL to fetch revoked items from
	URL string `yaml:"url"`

	// File path to read revoked items from
	File string `yaml:"file"`
}

// Validate performs validation on the OIDCRevocationSource configuration.
func (s *OIDCRevocationSource) Validate() error {
	return ValidateStruct(s,
		validation.Field(&s.URL, validation.When(s.File == "", validation.Required, is.URL)),
		validation.Field(&s.File, validation.When(s.URL == "", validation.Required)),
	)
}
