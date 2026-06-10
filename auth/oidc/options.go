// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas/core/types/redacted"
	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/observability/metrics"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
	httpclient "github.com/altessa-s/go-atlas/transport/http/client"
)

const (
	// DefaultJWKSHTTPTimeout is the timeout for JWKS HTTP requests.
	DefaultJWKSHTTPTimeout = 30 * time.Second

	// DefaultRevocationItemType is the default type of items stored in revocation storage.
	DefaultRevocationItemType = "token"

	// DefaultJWKSMaxStaleness is the default maximum age allowed for the
	// locally cached JWKS before validation reacts per the configured
	// [JWKSFailureMode]. A value of zero (the default) disables the
	// staleness check entirely — callers that want to enforce a freshness
	// bound must opt in via [WithJWKSMaxStaleness] and pair it with an
	// active refresh path ([WithJWKSRefreshSchedule] or manual
	// [Provider.RefreshJWKS] calls).
	DefaultJWKSMaxStaleness time.Duration = 0

	// DefaultJWKSFailureMode is the default behavior when the JWKS cache
	// exceeds [DefaultJWKSMaxStaleness]. Production safe: [JWKSFailureModeEnforce]
	// rejects validation, matching the rest of the project's safety-mode
	// pattern (see the plugins `SignatureMode` precedent).
	DefaultJWKSFailureMode = JWKSFailureModeEnforce
)

// JWKSFailureMode controls how the provider reacts when its locally cached
// JWKS has not been refreshed within [DefaultJWKSMaxStaleness] (or whatever
// the operator passed via [WithJWKSMaxStaleness]).
type JWKSFailureMode string

const (
	// JWKSFailureModeEnforce rejects validation with [ErrJWKSStale] once the
	// staleness threshold is crossed. This is the production-safe default —
	// it guarantees that a token whose signing key may have been rotated
	// upstream cannot be accepted on the strength of an unrefreshed cache.
	JWKSFailureModeEnforce JWKSFailureMode = "enforce"

	// JWKSFailureModeWarn logs an error every time validation observes a
	// stale JWKS cache but still allows the token through. Use this for
	// soft rollouts where rejecting traffic would be worse than serving
	// possibly-stale-keyed requests for a bounded window.
	JWKSFailureModeWarn JWKSFailureMode = "warn"

	// JWKSFailureModeDisabled bypasses the staleness check entirely. It is
	// equivalent to leaving [DefaultJWKSMaxStaleness] at zero and exists so
	// operators can override a non-zero default supplied through config
	// without recompiling.
	JWKSFailureModeDisabled JWKSFailureMode = "disabled"
)

// DefaultAudienceFailureMode is the default behavior when a token is validated
// without any expected audience configured (neither [WithValidationAudience]
// nor an `audience` entry in the service config). Production safe:
// [AudienceFailureModeEnforce] rejects validation, matching the rest of the
// project's safety-mode pattern (see [JWKSFailureMode] and the plugins
// `SignatureMode` precedent). The default-required `aud` claim only guarantees
// the token carries *an* audience — without an expected value to match against,
// a token minted for a different service replays successfully.
const DefaultAudienceFailureMode = AudienceFailureModeEnforce

// AudienceFailureMode controls how the provider reacts when token validation
// runs without any expected audience configured. When an expected audience is
// set, its value is always enforced regardless of this mode.
type AudienceFailureMode string

const (
	// AudienceFailureModeEnforce rejects validation with [ErrAudienceNotConfigured]
	// whenever no expected audience is configured. This is the production-safe
	// default — it prevents accepting a token minted for a different audience
	// (cross-audience replay) on the strength of the `aud` claim merely being
	// present.
	AudienceFailureModeEnforce AudienceFailureMode = "enforce"

	// AudienceFailureModeWarn logs an error every time validation runs without
	// an expected audience but still allows the token through. Use this for
	// soft rollouts where rejecting traffic would be worse than accepting
	// tokens whose audience cannot be bound to this service.
	AudienceFailureModeWarn AudienceFailureMode = "warn"

	// AudienceFailureModeDisabled bypasses the check entirely: tokens validate
	// without an expected audience and no diagnostic is emitted. Equivalent to
	// the pre-hardening behavior; opt in only when audience binding is provably
	// handled elsewhere.
	AudienceFailureModeDisabled AudienceFailureMode = "disabled"
)

// DefaultRequiredClaims is the list of claims required by OIDC specification.
var DefaultRequiredClaims = []string{"sub", "aud", "exp", "iat", "iss"}

// options holds the internal configuration state for the OIDC provider.
// This struct is not exported and is modified through Option functions.
type options struct {
	// httpClientOptions are forwarded to httpclient.New when the Provider
	// builds its outbound HTTP client. The factory layer uses this to
	// inject a proxy resolver materialized from config.HTTPProxy. Pass
	// httpclient.WithRetryMax(0) etc. here if the resilient defaults
	// (retry, breaker, env-proxy) are not desirable for a particular
	// deployment.
	httpClientOptions           []httpclient.Option `opt:"HTTPClientOptions" optgen:"append"`
	jwksHTTPTimeout             time.Duration       `optgen:"default=DefaultJWKSHTTPTimeout"`
	jwksMaxStaleness            time.Duration       `opt:"JWKSMaxStaleness" optgen:"default=DefaultJWKSMaxStaleness"`
	jwksFailureMode             JWKSFailureMode     `optgen:"manual,default=DefaultJWKSFailureMode"`
	audienceFailureMode         AudienceFailureMode `optgen:"manual,default=DefaultAudienceFailureMode"`
	tokenCache                  Cacher
	tokensCacheKeyPrefix        string `optgen:"default=DefaultTokensCacheKeyPrefix"`
	revokedTokensCacheKeyPrefix string `optgen:"default=DefaultRevokedTokensCacheKeyPrefix"`
	activeTokensCacheKeyPrefix  string `optgen:"default=DefaultActiveTokensCacheKeyPrefix"`
	serviceConfigPath           string
	logger                      *slog.Logger
	introspectionEnabled        bool                         `opt:"-"`
	introspectionClientID       string                       `opt:"-"`
	introspectionSecret         redacted.RedactedString      `opt:"-"`
	introspectionFailOpen       bool                         `opt:"-"`
	verifierOptions             *verifierOptions             `opt:"-"`
	presets                     map[string]*ValidationPreset `opt:"-"`
	presetRules                 []PresetRule
	revocationStorage           RevocationStorage
	revocationItemType          string     `optgen:"default=DefaultRevocationItemType"`
	revocationFilter            Filter     `optgen:"notnil"`
	revocationLoader            DataLoader `optgen:"notnil"`

	// Health check configuration
	healthCoordinator *health.Coordinator

	// Scheduler configuration
	scheduler              corescheduler.TaskRegistrar `optgen:"notnil"`
	jwksRefreshEnabled     bool                        `opt:"-"`
	jwksRefreshSchedule    string                      `opt:"-"`
	revocationSyncEnabled  bool                        `opt:"-"`
	revocationSyncSchedule string                      `opt:"-"`

	// Metrics collector for Prometheus-compatible instrumentation.
	collector metrics.Collector `optgen:"notnil"`
}

// WithDefaultValidationOptions sets default validation options for ValidateToken.
// These apply to all validations unless overridden by per-call options.
func WithDefaultValidationOptions(vopt ...ValidationOption) Option {
	return func(o *options) {
		verifierOpts := &verifierOptions{}
		for _, opt := range vopt {
			opt(verifierOpts)
		}
		o.verifierOptions = verifierOpts
	}
}

// WithIntrospection enables RFC 7662 token introspection for revocation checks.
// Requires client credentials; results are cached for performance.
//
// Introspection is fail-closed by default: when the endpoint cannot confirm a
// token is active (network error, non-2xx response, malformed payload) the
// token is rejected with [ErrIntrospection]. This keeps revocation enforced
// under IdP degradation. Opt into fail-open behavior with
// [WithIntrospectionFailOpen] only when availability must be preferred over
// revocation guarantees — be aware that a degraded IdP then silently bypasses
// revocation.
func WithIntrospection(clientID, clientSecret string) Option {
	return func(o *options) {
		o.introspectionEnabled = true
		o.introspectionClientID = clientID
		o.introspectionSecret = redacted.RedactedString(clientSecret)
	}
}

// WithIntrospectionFailOpen relaxes introspection to fail-open: when the
// configured endpoint cannot confirm a token is active (network error, non-2xx
// response, parse failure) the error is logged and the token is accepted on
// its signature alone. Use this only when IdP availability must take
// precedence over revocation enforcement — under this mode a degraded or
// unreachable IdP silently bypasses revocation checks.
//
// Without this option introspection is fail-closed (the secure default):
// errors reject the token with [ErrIntrospection].
func WithIntrospectionFailOpen() Option {
	return func(o *options) {
		o.introspectionFailOpen = true
	}
}

// WithPresets registers named validation presets for reusable rules.
// Presets are stored by name and can be referenced during validation.
func WithPresets(presets ...*ValidationPreset) Option {
	return func(o *options) {
		if o.presets == nil {
			o.presets = make(map[string]*ValidationPreset)
		}
		for _, preset := range presets {
			if preset != nil {
				o.presets[preset.name] = preset
			}
		}
	}
}

// WithJWKSRefreshSchedule configures JWKS refresh task for scheduler.
// This task will be registered if scheduler is provided via WithScheduler.
// The schedule parameter should be a cron expression (e.g., "0 */30 * * * *" for every 30 minutes).
func WithJWKSRefreshSchedule(schedule string) Option {
	return func(o *options) {
		o.jwksRefreshEnabled = true
		o.jwksRefreshSchedule = schedule
	}
}

// WithJWKSFailureMode selects how the provider reacts when its locally
// cached JWKS has not been refreshed within [WithJWKSMaxStaleness].
// Defaults to [JWKSFailureModeEnforce]. Unknown / empty modes leave the
// default in place.
//
// Pair this with [WithJWKSMaxStaleness] (and an active refresh path such
// as [WithJWKSRefreshSchedule] or manual [Provider.RefreshJWKS] calls);
// without a non-zero staleness budget the mode has no effect.
func WithJWKSFailureMode(mode JWKSFailureMode) Option {
	return func(o *options) {
		switch mode {
		case JWKSFailureModeEnforce, JWKSFailureModeWarn, JWKSFailureModeDisabled:
			o.jwksFailureMode = mode
		}
	}
}

// WithAudienceFailureMode selects how the provider reacts when token
// validation runs without any expected audience configured. Defaults to
// [AudienceFailureModeEnforce]. Unknown / empty modes leave the default in
// place. When an expected audience is configured (via [WithValidationAudience]
// or service config) its value is always enforced regardless of this mode.
func WithAudienceFailureMode(mode AudienceFailureMode) Option {
	return func(o *options) {
		switch mode {
		case AudienceFailureModeEnforce, AudienceFailureModeWarn, AudienceFailureModeDisabled:
			o.audienceFailureMode = mode
		}
	}
}

// WithRevocationSyncSchedule configures revocation sync task for scheduler.
// This task will be registered if scheduler is provided via WithScheduler.
// The schedule parameter should be a cron expression (e.g., "0 */5 * * * *" for every 5 minutes).
func WithRevocationSyncSchedule(schedule string) Option {
	return func(o *options) {
		o.revocationSyncEnabled = true
		o.revocationSyncSchedule = schedule
	}
}
