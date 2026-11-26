// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

//go:generate go tool optgen generate --type=validatorOptions --option-type=ValidatorOption --output=validator_options_gen.go --option-prefix=Validator
//go:generate go tool optgen generate --type=verifierOptions --option-type=ValidationOption --output=verifier_options_gen.go --option-prefix=Validation
//go:generate go tool optgen generate

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/altessa-s/go-atlas-core/types/redacted"
	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/observability/metrics"

	probfilter "github.com/altessa-s/go-atlas/cache/probfilter"
)

const (
	// DefaultJWKSHTTPTimeout is the timeout for JWKS HTTP requests.
	DefaultJWKSHTTPTimeout = 30 * time.Second

	// DefaultRevocationItemType is the default type of items stored in revocation storage.
	DefaultRevocationItemType = "token"

	// DefaultRevocationInitialSyncWait bounds how long NewProvider keeps
	// retrying its initial revocation sync while a shared filter is being
	// rebuilt by another replica (or this rebuild lost its lease) before the
	// configured fail mode applies. Every replica completes one sync of its
	// own and the rebuild lease serializes them, so when N replicas start at
	// once the last one waits about (N-1) × the rebuild duration: keep the
	// wait above that, or limit how many replicas start together.
	DefaultRevocationInitialSyncWait = 2 * time.Minute

	// DefaultRevocationMaxStaleness is the default bound on the age of the last
	// successful revocation sync — see [WithRevocationMaxStaleness]. The sync
	// schedule is a cron expression with no fixed interval to multiply, so the
	// bound is a constant: three periods of the default hourly schedule
	// ("0 0 * * * *"), which tolerates two missed syncs. Set the bound to
	// several sync intervals when the schedule is sparser.
	DefaultRevocationMaxStaleness = 3 * time.Hour

	// DefaultRevocationSyncTimeout bounds one scheduled revocation sync — see
	// [WithRevocationSyncTimeout].
	DefaultRevocationSyncTimeout = 5 * time.Minute

	// DefaultRevocationMaxResponseBytes is the default cap on the body a
	// [URLRevocationLoader] reads (256 MiB).
	DefaultRevocationMaxResponseBytes int64 = 256 << 20

	// MaxRevocationEntryBytes is the longest revocation list entry (one line)
	// the built-in loaders accept. A longer line fails the whole load.
	MaxRevocationEntryBytes = 64 << 10

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
	// pattern.
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
// project's safety-mode pattern (see [JWKSFailureMode]). The default-required `aud` claim only guarantees
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

// DefaultDiscoveryValidationMode is the default handling of the discovery
// document's issuer and endpoint URLs. Production safe:
// [DiscoveryValidationModeEnforce] rejects a discovery response whose issuer
// does not match the configured URL, or whose endpoints are non-HTTPS or
// cross-origin — the mix-up / SSRF defense described on
// [DiscoveryValidationMode].
const DefaultDiscoveryValidationMode = DiscoveryValidationModeEnforce

// DiscoveryValidationMode controls how the provider validates the OIDC
// discovery document before trusting the endpoint URLs it advertises
// (jwks_uri, token_endpoint, introspection_endpoint, userinfo_endpoint,
// authorization_endpoint).
//
// Without validation a malicious or compromised IdP — or a network attacker
// on a non-HTTPS discovery fetch — can point those endpoints at hosts it
// controls, exfiltrating the introspection client secret (sent as HTTP Basic
// auth) and live user access tokens (sent as Bearer), or driving SSRF against
// internal addresses. Enforce mode closes this by requiring:
//   - the document's issuer to equal the configured issuer (RFC 8414 §3.3 /
//     OIDC Discovery §4.3 exact match);
//   - every advertised endpoint to use https;
//   - every advertised endpoint to share the issuer's host.
type DiscoveryValidationMode string

const (
	// DiscoveryValidationModeEnforce rejects discovery with
	// [ErrDiscoveryValidation] when the issuer mismatches or any endpoint is
	// non-HTTPS or cross-origin. Recommended for production.
	DiscoveryValidationModeEnforce DiscoveryValidationMode = "enforce"

	// DiscoveryValidationModeWarn logs an error for every violation but loads
	// the discovery document anyway. Use only during migration while auditing
	// a non-conforming IdP.
	DiscoveryValidationModeWarn DiscoveryValidationMode = "warn"

	// DiscoveryValidationModeDisabled bypasses issuer/endpoint validation
	// entirely: the discovery response is trusted as-is. Equivalent to the
	// pre-hardening behavior; opt in only for a loopback/test IdP or when the
	// discovery transport is provably attacker-proof.
	DiscoveryValidationModeDisabled DiscoveryValidationMode = "disabled"
)

// DefaultRequiredClaims is the list of claims required by OIDC specification.
// It is always enforced: [WithValidationRequiredClaims] adds claims to this
// set, and [WithValidationIgnoredClaims] can skip any of them except exp.
var DefaultRequiredClaims = []string{"sub", "aud", "exp", "iat", "iss"}

// options holds the internal configuration state for the OIDC provider.
// This struct is not exported and is modified through Option functions.
type options struct {
	// Shared HTTP client and instrumentation.

	// httpClient serves every outbound call (discovery, JWKS, introspection,
	// userinfo, revocation). Required: NewProvider fails with
	// ErrHTTPClientRequired without it. The factory injects the resilient,
	// SSRF-protected atlas client (retry, breaker, proxy) built from
	// configuration.
	httpClient *http.Client `opt:"HTTPClient"`
	// WithOwnedHTTPClient hands the client passed with [WithHTTPClient] over
	// to the provider: [Provider.Close] then closes the idle connections of
	// its transport. Set it only for a client built for this provider alone
	// (the factory does). Without it the client stays the caller's — it may
	// be shared with other components — and Close leaves its connections
	// alone.
	httpClientOwned bool `opt:"OwnedHTTPClient"`
	logger          *slog.Logger
	// Metrics meter for Prometheus-compatible instrumentation.
	meter metrics.Meter `optgen:"notnil"`
	// Health check configuration
	healthCoordinator health.Registrar `optgen:"notnil"`

	// Discovery.

	// WithDiscoveryValidationMode selects how the provider validates the OIDC
	// discovery document's issuer and endpoint URLs before trusting them.
	// Defaults to [DiscoveryValidationModeEnforce]. Unknown / empty modes leave
	// the default in place. See [DiscoveryValidationMode] for the threat model.
	discoveryValidationMode DiscoveryValidationMode `opt:"DiscoveryValidationMode" optcheck:"oneof=[DiscoveryValidationModeEnforce,DiscoveryValidationModeWarn,DiscoveryValidationModeDisabled]" optgen:"default=DefaultDiscoveryValidationMode,onfail=skip"` //nolint:lll
	// discoveryAllowLoopbackHTTP lets enforce and warn validation accept a
	// plain-http issuer and endpoints when the issuer host is a loopback
	// address (localhost, 127.0.0.0/8, ::1) — a local IdP in development or
	// tests. The issuer match and the same-host rule still apply, so an
	// endpoint can only be http on the issuer's own loopback host.
	discoveryAllowLoopbackHTTP bool `opt:"DiscoveryAllowLoopbackHTTP"`

	// JWKS refresh and freshness.

	jwksHTTPTimeout  time.Duration `optgen:"default=DefaultJWKSHTTPTimeout"`
	jwksMaxStaleness time.Duration `opt:"JWKSMaxStaleness" optgen:"default=DefaultJWKSMaxStaleness"`
	// WithJWKSFailureMode selects how the provider reacts when its locally
	// cached JWKS has not been refreshed within [WithJWKSMaxStaleness].
	// Defaults to [JWKSFailureModeEnforce]. Unknown / empty modes leave the
	// default in place.
	//
	// Pair this with [WithJWKSMaxStaleness] (and an active refresh path such
	// as [WithJWKSRefreshSchedule] or manual [Provider.RefreshJWKS] calls);
	// without a non-zero staleness budget the mode has no effect.
	jwksFailureMode JWKSFailureMode `opt:"JWKSFailureMode" optcheck:"oneof=[JWKSFailureModeEnforce,JWKSFailureModeWarn,JWKSFailureModeDisabled]" optgen:"default=DefaultJWKSFailureMode,onfail=skip"` //nolint:lll
	// Set by WithJWKSRefreshSchedule: enables the process-local refresh task and sets its schedule.
	jwksRefreshEnabled bool `optgen:"manual"`
	// Set by WithJWKSRefreshSchedule: enables the process-local refresh task and sets its schedule.
	jwksRefreshSchedule string `optgen:"manual"`

	// Policy evaluation and signature caching.

	// WithAudienceFailureMode selects how the provider reacts when token
	// validation runs without any expected audience configured. Defaults to
	// [AudienceFailureModeEnforce]. Unknown / empty modes leave the default in
	// place. When an expected audience is configured (via [WithValidationAudience]
	// or service config) its value is always enforced regardless of this mode.
	audienceFailureMode AudienceFailureMode `opt:"AudienceFailureMode" optcheck:"oneof=[AudienceFailureModeEnforce,AudienceFailureModeWarn,AudienceFailureModeDisabled]" optgen:"default=DefaultAudienceFailureMode,onfail=skip"` //nolint:lll
	// WithDefaultValidationOptions sets default validation options for ValidateToken.
	// These apply to all validations unless overridden by per-call options.
	verifierOptions *verifierOptions `optfn:"DefaultValidationOptions=newVerifierOptions"`
	// Set by WithPresets: filters nil presets and indexes the values by name.
	presets              map[string]*ValidationPreset `optgen:"manual"`
	presetRules          []PresetRule
	tokenCache           Cacher
	tokensCacheKeyPrefix string `optgen:"default=DefaultTokensCacheKeyPrefix"`

	// Introspection and revocation.

	// Set by WithIntrospection: enables introspection and stores its credentials together.
	introspectionEnabled bool `optgen:"manual"`
	// Set by WithIntrospection: enables introspection and stores its credentials together.
	introspectionClientID string `optgen:"manual"`
	// Set by WithIntrospection: enables introspection and stores its credentials together.
	introspectionSecret redacted.RedactedString `optgen:"manual"`
	// WithIntrospectionFailOpen relaxes introspection to fail-open: when the
	// configured endpoint cannot confirm a token is active (network error, non-2xx
	// response, parse failure) the error is logged and the token is accepted on
	// its signature alone. Use this only when IdP availability must take
	// precedence over revocation enforcement — under this mode a degraded or
	// unreachable IdP silently bypasses revocation checks.
	//
	// Without this option introspection is fail-closed (the secure default):
	// errors reject the token with [ErrIntrospection].
	introspectionFailOpen       bool   `opt:"IntrospectionFailOpen"`
	revokedTokensCacheKeyPrefix string `optgen:"default=DefaultRevokedTokensCacheKeyPrefix"`
	activeTokensCacheKeyPrefix  string `optgen:"default=DefaultActiveTokensCacheKeyPrefix"`
	revocationStorage           RevocationStorage
	revocationItemType          string            `optgen:"default=DefaultRevocationItemType"`
	revocationFilter            probfilter.Filter `optgen:"notnil"`
	// WithRevocationFilterCleanup transfers cleanup of a dedicated revocation filter
	// to NewProvider. It runs on construction failure or once during Close.
	// Omit it for a borrowed filter or storage.
	revocationFilterCleanup func(context.Context) error
	revocationLoader        probfilter.DataLoader `optgen:"notnil"`
	revocationAuthoritative Authoritative         `optgen:"notnil"`
	// revocationFailOpen accepts a token when the revocation storage cannot
	// answer (IsRevoked error) and lets NewProvider start when the initial
	// revocation sync fails. The default (false) is fail-closed: such a token
	// is rejected with ErrRevocationCheck and construction fails.
	revocationFailOpen bool
	// revocationSkipMissingItem looks a token up by the whole token when the
	// configured item claim (jti or kid) is missing or not a string, instead of
	// rejecting it with ErrTokenInvalid. Such a token can never match an item
	// revoked by jti or kid, so it is accepted unchecked.
	revocationSkipMissingItem bool `opt:"InsecureRevocationSkipMissingItem"`
	// revocationInitialSyncWait bounds the initial-sync retries — see
	// DefaultRevocationInitialSyncWait.
	revocationInitialSyncWait time.Duration `optgen:"default=DefaultRevocationInitialSyncWait"`
	// WithRevocationMaxStaleness bounds how long a "not revoked" answer of the
	// revocation storage is trusted after its last successful
	// [RevocationStorage.Sync]. Past the bound such an answer counts as a failed
	// check: the token is rejected with [ErrRevocationCheck] wrapping
	// [ErrRevocationStale] unless [WithRevocationFailOpen] is set. A revoked
	// answer always rejects.
	//
	// The bound defaults to [DefaultRevocationMaxStaleness] and applies only
	// when the provider syncs the storage on [WithRevocationSyncSchedule]:
	// without a scheduled sync nothing refreshes the snapshot, so nothing is
	// stale. Keep it at several sync intervals. After a failed initial sync
	// under [WithRevocationFailOpen] the bound counts from construction.
	//
	// Zero or a negative value disables the bound: a sync source that stays
	// unreachable then leaves the last snapshot in force indefinitely.
	revocationMaxStaleness time.Duration `optgen:"default=DefaultRevocationMaxStaleness" optval:"nonpositive"`
	// WithRevocationSyncTimeout bounds one scheduled revocation sync
	// ([WithRevocationSyncSchedule]); a sync that runs longer fails and keeps
	// the current revocation set. Defaults to [DefaultRevocationSyncTimeout].
	// Zero disables the timeout; a negative value is ignored. The initial sync
	// in [NewProvider] is bounded by the constructor's context instead.
	revocationSyncTimeout time.Duration `optgen:"default=DefaultRevocationSyncTimeout" optval:"positive=allow_zero"`
	// WithRevocationAllowEmpty makes an empty result of a revocation sync
	// authoritative for the built-in filter storage ([WithRevocationFilter],
	// [NewFilterRevocationStorage]): the filter is rebuilt empty and the sync
	// succeeds, which also refreshes the [WithRevocationMaxStaleness] clock.
	// Set it when the revocation list can legitimately become empty.
	//
	// Without it (the default) a source that yields no entries while the
	// revocation set holds some fails the sync with [ErrRevocationLoadFailed]
	// and keeps the current set. With it that protection is gone: a source
	// that answers 200 with an empty or blank body — an IdP outage, a
	// misrouted request, a truncated file — un-revokes every item at once.
	// A custom [RevocationStorage] decides this on its own.
	revocationAllowEmpty bool
	// Set by WithRevocationSyncSchedule: enables the process-local sync task and sets its schedule.
	revocationSyncEnabled bool `optgen:"manual"`
	// Set by WithRevocationSyncSchedule: enables the process-local sync task and sets its schedule.
	revocationSyncSchedule string `optgen:"manual"`
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

// WithJWKSRefreshSchedule refreshes the JWKS keys on the provider's own,
// process-local cron. The schedule is a six-field cron expression with seconds
// (e.g. "0 */30 * * * *" for every 30 minutes) or a descriptor such as
// "@every 30m"; an invalid expression fails [NewProvider].
func WithJWKSRefreshSchedule(schedule string) Option {
	return func(o *options) {
		o.jwksRefreshEnabled = true
		o.jwksRefreshSchedule = schedule
	}
}

// WithRevocationSyncSchedule syncs the revocation storage
// ([RevocationStorage.Sync]) on the provider's own, process-local cron, on top
// of the sync NewProvider runs at construction. The schedule is a six-field
// cron expression with seconds (e.g. "0 */5 * * * *" for every 5 minutes) or
// a descriptor such as "@every 5m"; an invalid expression fails [NewProvider].
// It has no effect without a revocation storage. Each run is bounded by
// [WithRevocationSyncTimeout], and the schedule turns on the
// [WithRevocationMaxStaleness] bound.
func WithRevocationSyncSchedule(schedule string) Option {
	return func(o *options) {
		o.revocationSyncEnabled = true
		o.revocationSyncSchedule = schedule
	}
}
