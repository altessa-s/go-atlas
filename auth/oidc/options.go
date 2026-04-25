// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

import (
	"log/slog"
	"time"

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
	tokenCache                  Cacher
	tokensCacheKeyPrefix        string `optgen:"default=DefaultTokensCacheKeyPrefix"`
	revokedTokensCacheKeyPrefix string `optgen:"default=DefaultRevokedTokensCacheKeyPrefix"`
	activeTokensCacheKeyPrefix  string `optgen:"default=DefaultActiveTokensCacheKeyPrefix"`
	serviceConfigPath           string
	logger                      *slog.Logger
	introspectionEnabled        bool                         `opt:"-"`
	introspectionClientID       string                       `opt:"-"`
	introspectionSecret         string                       `opt:"-"`
	introspectionStrict         bool                         `opt:"-"`
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
// By default introspection failures (network errors, 5xx responses, malformed
// payloads) are logged and the token is accepted on signature validation alone
// — fail-open. Pair this with [WithIntrospectionStrict] in production to
// reject tokens whenever the introspection endpoint is unreachable, otherwise
// a degraded IdP silently bypasses revocation.
func WithIntrospection(clientID, clientSecret string) Option {
	return func(o *options) {
		o.introspectionEnabled = true
		o.introspectionClientID = clientID
		o.introspectionSecret = clientSecret
	}
}

// WithIntrospectionStrict makes [Provider.ValidateToken] reject tokens with
// [ErrIntrospection] whenever the configured introspection endpoint cannot
// confirm the token is active (network error, non-2xx response, parse
// failure). Use this in production to keep revocation enforced under IdP
// degradation; combine with [WithIntrospection] which provides the
// credentials.
//
// Without this option introspection is fail-open: errors are logged and the
// token is accepted if its signature is valid.
func WithIntrospectionStrict() Option {
	return func(o *options) {
		o.introspectionStrict = true
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

// WithRevocationSyncSchedule configures revocation sync task for scheduler.
// This task will be registered if scheduler is provided via WithScheduler.
// The schedule parameter should be a cron expression (e.g., "0 */5 * * * *" for every 5 minutes).
func WithRevocationSyncSchedule(schedule string) Option {
	return func(o *options) {
		o.revocationSyncEnabled = true
		o.revocationSyncSchedule = schedule
	}
}
