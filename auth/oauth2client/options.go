// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/oauth2"
)

// DefaultAuthStyle lets x/oauth2 auto-detect whether client credentials are
// sent in the Authorization header or the request body. Override with
// [WithAuthStyle] when talking to an IdP that accepts only one style.
const DefaultAuthStyle = oauth2.AuthStyleAutoDetect

// Retry defaults for [Exchanger]. Retrying is opt-in: the zero
// [DefaultRetryAttempts] performs a single attempt, preserving the no-retry
// behavior of the other helpers (x/oauth2 does not retry either).
const (
	// DefaultRetryAttempts is the number of retries after the first attempt.
	// Zero disables retrying. Raise it with [WithRetryAttempts].
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

// options carries the tunables shared by every grant helper. All fields are
// optional; the zero value talks to a spec-compliant IdP over
// [http.DefaultClient] with credentials auto-detected.
type options struct {
	// scopes are the requested OAuth2 scopes, sent as the space-delimited
	// scope parameter. Nil requests no explicit scopes.
	scopes []string

	// authStyle selects how client credentials are presented to the token
	// endpoint (header vs. body). It applies to [ClientCredentials], [Refresh],
	// and [Exchanger]; the authorization-code endpoint carries its own style on
	// [oauth2.Endpoint].
	authStyle oauth2.AuthStyle `optgen:"default=DefaultAuthStyle"`

	// endpointParams are extra, non-standard parameters added to every token
	// request (for example Auth0's "audience"). It applies to
	// [ClientCredentials] and [Exchanger].
	endpointParams url.Values

	// httpClient issues the token-endpoint requests. Nil uses
	// [http.DefaultClient]. Inject a client with timeouts, mTLS, or tracing
	// round-trippers here.
	httpClient *http.Client

	// retryAttempts is the number of retries after the first [Exchanger]
	// attempt on a transient failure (transport error, 429, or 5xx). Zero
	// disables retrying. It applies only to [Exchanger]; the x/oauth2-backed
	// helpers manage their own transport.
	retryAttempts int `optgen:"default=DefaultRetryAttempts"`

	// retryBaseDelay and retryMaxDelay bound the exponential backoff between
	// [Exchanger] retries.
	retryBaseDelay time.Duration `optgen:"default=DefaultRetryBaseDelay"`
	retryMaxDelay  time.Duration `optgen:"default=DefaultRetryMaxDelay"`

	// metrics records fetch counts, latency, and retry counts across the flows.
	// Nil disables metric recording.
	metrics *Metrics

	// logger records token-fetch failures and retry attempts. Nil disables
	// logging (no default handler is installed, keeping the hot path free of a
	// wrapping token source).
	logger *slog.Logger

	// earlyExpiry makes a self-refreshing source treat a token as expired this
	// long before its actual exp, so a fresh token is fetched ahead of expiry to
	// absorb clock skew and in-flight latency. Zero uses x/oauth2's ~10s default.
	// It applies to [ClientCredentials] and [Exchanger.TokenSource]; [Refresh]
	// and [AuthCode] keep the x/oauth2 default (overriding it would drop
	// refresh-token rotation).
	earlyExpiry time.Duration

	// clientAuth authenticates the client with a signed JWT assertion
	// (private_key_jwt / client_secret_jwt), superseding the secret-based
	// [WithAuthStyle] path. It applies to [ClientCredentials] and [Exchanger].
	// Set via the generated WithClientAuth.
	clientAuth ClientAuthenticator `optgen:"notnil"`
}

// withHTTPClient returns ctx carrying client as the [oauth2.HTTPClient] value
// x/oauth2 reads, or ctx unchanged when client is nil (x/oauth2 then falls back
// to [http.DefaultClient]).
func withHTTPClient(ctx context.Context, client *http.Client) context.Context {
	if client == nil {
		return ctx
	}
	return context.WithValue(ctx, oauth2.HTTPClient, client)
}
