// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client

//go:generate go tool optgen generate --type=clientCredentialsOptions --option-type=ClientCredentialsOption --output=client_credentials_options_gen.go --option-prefix=ClientCredentials
//go:generate go tool optgen generate --type=refreshOptions --option-type=RefreshOption --output=refresh_options_gen.go --option-prefix=Refresh
//go:generate go tool optgen generate --type=authCodeOptions --option-type=AuthCodeOption --output=auth_code_options_gen.go --option-prefix=AuthCode
//go:generate go tool optgen generate --type=deviceFlowOptions --option-type=DeviceFlowOption --output=device_flow_options_gen.go --option-prefix=DeviceFlow
//go:generate go tool optgen generate --type=exchangerOptions --option-type=ExchangerOption --output=exchanger_options_gen.go --option-prefix=Exchanger
//go:generate go tool optgen generate --type=jwtBearerOptions --option-type=JWTBearerOption --output=jwt_bearer_options_gen.go --option-prefix=JWTBearer --new-func-name=newJWTBearerOptions
//go:generate go tool optgen generate --type=revokerOptions --option-type=RevokerOption --output=revoker_options_gen.go --option-prefix=Revoker

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/altessa-s/go-atlas-core/time/clock"

	"golang.org/x/oauth2"
)

// DefaultAuthStyle lets x/oauth2 auto-detect whether client credentials are
// sent in the Authorization header or the request body. Override with
// the grant-specific auth style when talking to an IdP that accepts only one style.
const DefaultAuthStyle = oauth2.AuthStyleAutoDetect

// Retry defaults for [Exchanger]. Retrying is opt-in: the zero
// [DefaultRetryAttempts] performs a single attempt, preserving the no-retry
// behavior of the other helpers (x/oauth2 does not retry either).
const (
	// DefaultRetryAttempts is the number of retries after the first attempt.
	// Zero disables retrying. Raise it with [WithExchangerRetryAttempts].
	DefaultRetryAttempts = 0

	// DefaultRetryBaseDelay is the backoff applied before the first retry.
	DefaultRetryBaseDelay = 200 * time.Millisecond

	// DefaultRetryMaxDelay caps the exponential backoff between retries.
	DefaultRetryMaxDelay = 5 * time.Second

	// DefaultRetryJitter randomizes each backoff delay by up to this fraction
	// of itself. A token endpoint is a single dependency shared by every
	// instance of every service that talks to it, so an outage synchronizes
	// their retries; without jitter the endpoint is hit by waves at exactly
	// the moments it is trying to recover.
	DefaultRetryJitter = 0.2
)

type clientCredentialsOptions struct {
	// WithClientCredentialsScopes sets the requested OAuth2 scopes.
	scopes []string
	// WithClientCredentialsAuthStyle selects how the client presents its credentials to the endpoint.
	authStyle oauth2.AuthStyle `optgen:"default=DefaultAuthStyle"`
	// WithClientCredentialsEndpointParams sets additional token request parameters.
	endpointParams url.Values
	// WithClientCredentialsHTTPClient sets the HTTP client used for endpoint requests.
	httpClient *http.Client
	// WithClientCredentialsInsecureHTTP permits plain HTTP endpoints on non-loopback hosts.
	insecureHTTP bool `opt:"InsecureHTTP"`
	// WithClientCredentialsMetrics records token fetch metrics.
	metrics *Metrics
	// WithClientCredentialsLogger logs token fetch failures.
	logger *slog.Logger
	// WithClientCredentialsEarlyExpiry sets how far ahead of expiry to refresh a cached token.
	earlyExpiry time.Duration
	// WithClientCredentialsClientAuth authenticates the client with a signed assertion.
	clientAuth ClientAuthenticator `optgen:"notnil"`
}

type refreshOptions struct {
	// WithRefreshAuthStyle selects how the client presents its credentials to the endpoint.
	authStyle oauth2.AuthStyle `optgen:"default=DefaultAuthStyle"`
	// WithRefreshHTTPClient sets the HTTP client used for endpoint requests.
	httpClient *http.Client
	// WithRefreshInsecureHTTP permits plain HTTP endpoints on non-loopback hosts.
	insecureHTTP bool `opt:"InsecureHTTP"`
	// WithRefreshMetrics records token fetch metrics.
	metrics *Metrics
	// WithRefreshLogger logs token fetch failures.
	logger *slog.Logger
}

type authCodeOptions struct {
	// WithAuthCodeScopes sets the requested OAuth2 scopes.
	scopes []string
	// WithAuthCodeHTTPClient sets the HTTP client used for endpoint requests.
	httpClient *http.Client
	// WithAuthCodeInsecureHTTP permits plain HTTP endpoints on non-loopback hosts.
	insecureHTTP bool `opt:"InsecureHTTP"`
	// WithAuthCodeMetrics records token fetch metrics.
	metrics *Metrics
	// WithAuthCodeLogger logs token fetch failures.
	logger *slog.Logger
}

type deviceFlowOptions struct {
	// WithDeviceFlowScopes sets the requested OAuth2 scopes.
	scopes []string
	// WithDeviceFlowHTTPClient sets the HTTP client used for endpoint requests.
	httpClient *http.Client
	// WithDeviceFlowInsecureHTTP permits plain HTTP endpoints on non-loopback hosts.
	insecureHTTP bool `opt:"InsecureHTTP"`
	// WithDeviceFlowMetrics records token fetch metrics.
	metrics *Metrics
	// WithDeviceFlowLogger logs token fetch failures.
	logger *slog.Logger
}

type exchangerOptions struct {
	// WithExchangerAuthStyle selects how the client presents its credentials to the endpoint.
	authStyle oauth2.AuthStyle `optgen:"default=DefaultAuthStyle"`
	// WithExchangerEndpointParams sets additional token request parameters.
	endpointParams url.Values
	// WithExchangerHTTPClient sets the HTTP client used for endpoint requests.
	httpClient *http.Client
	// WithExchangerInsecureHTTP permits plain HTTP endpoints on non-loopback hosts.
	insecureHTTP bool `opt:"InsecureHTTP"`
	// WithExchangerRetryAttempts sets the number of retries after a transient failure.
	retryAttempts int `optgen:"default=DefaultRetryAttempts"`
	// WithExchangerRetryBaseDelay sets the initial retry delay.
	retryBaseDelay time.Duration `optgen:"default=DefaultRetryBaseDelay"`
	// WithExchangerRetryMaxDelay caps the retry delay.
	retryMaxDelay time.Duration `optgen:"default=DefaultRetryMaxDelay"`
	// WithExchangerMetrics records token fetch metrics.
	metrics *Metrics
	// WithExchangerLogger logs token fetch failures.
	logger *slog.Logger
	// WithExchangerEarlyExpiry sets how far ahead of expiry to refresh a cached token.
	earlyExpiry time.Duration
	// WithExchangerClientAuth authenticates the client with a signed assertion.
	clientAuth ClientAuthenticator `optgen:"notnil"`
}

type jwtBearerOptions struct {
	// WithJWTBearerScopes sets the requested OAuth2 scopes.
	scopes []string
	// WithJWTBearerHTTPClient sets the HTTP client used for endpoint requests.
	httpClient *http.Client
	// WithJWTBearerInsecureHTTP permits plain HTTP endpoints on non-loopback hosts.
	insecureHTTP bool `opt:"InsecureHTTP"`
	// WithJWTBearerRetryAttempts sets the number of retries after a transient failure.
	retryAttempts int `optgen:"default=DefaultRetryAttempts"`
	// WithJWTBearerRetryBaseDelay sets the initial retry delay.
	retryBaseDelay time.Duration `optgen:"default=DefaultRetryBaseDelay"`
	// WithJWTBearerRetryMaxDelay caps the retry delay.
	retryMaxDelay time.Duration `optgen:"default=DefaultRetryMaxDelay"`
	// WithJWTBearerMetrics records token fetch metrics.
	metrics *Metrics
	// WithJWTBearerLogger logs token fetch failures.
	logger *slog.Logger
	// WithJWTBearerEarlyExpiry sets how far ahead of expiry to refresh a cached token.
	earlyExpiry time.Duration
	// WithJWTBearerClock sets the clock used for assertions and token expiry.
	clock clock.Clock `optgen:"default=clock.Real(),notnil"`
}

type revokerOptions struct {
	// WithRevokerAuthStyle selects how the client presents its credentials to the endpoint.
	authStyle oauth2.AuthStyle `optgen:"default=DefaultAuthStyle"`
	// WithRevokerHTTPClient sets the HTTP client used for endpoint requests.
	httpClient *http.Client
	// WithRevokerInsecureHTTP permits plain HTTP endpoints on non-loopback hosts.
	insecureHTTP bool `opt:"InsecureHTTP"`
	// WithRevokerClientAuth authenticates the client with a signed assertion.
	clientAuth ClientAuthenticator `optgen:"notnil"`
}

// withHTTPClient returns ctx carrying client as the [oauth2.HTTPClient] value
// x/oauth2 reads, or ctx unchanged when client is nil (x/oauth2 then falls back
// to [http.DefaultClient]). Every flow passes a client from [endpointClient],
// which is never nil.
func withHTTPClient(ctx context.Context, client *http.Client) context.Context {
	if client == nil {
		return ctx
	}
	return context.WithValue(ctx, oauth2.HTTPClient, client)
}
