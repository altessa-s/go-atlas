// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client

import "errors"

// Sentinel errors returned by [Exchanger] and the other helpers. Errors from
// [ClientCredentials], [Refresh], and [AuthCode] carry the underlying
// x/oauth2 error (typically an [*oauth2.RetrieveError]), reachable via
// errors.As, so callers can inspect the IdP's status code and body — except
// [ClientCredentials] with a [ClientAuthenticator], which wraps
// [ErrTokenRequest] or [ErrTokenExchange] (see [ErrTokenRequest]).
//
// Errors for parsed responses leave the IdP response body out, because it
// may echo request parameters. Their text names the HTTP status and an
// RFC 6749/8628 error code from the fixed allowlist, with only the lengths of
// error_description and error_uri, or only the body length without a usable code.
var (
	// ErrTokenExchange wraps a failed RFC 8693 token exchange: a transport
	// error, a non-2xx response from the token endpoint, or an unparsable body.
	// The IdP's status and error code are attached to the message.
	ErrTokenExchange = errors.New("auth/oauth2client: token exchange failed")

	// ErrSubjectTokenRequired is returned by [Exchanger.Exchange] when the
	// [ExchangeRequest] carries an empty SubjectToken. RFC 8693 requires it.
	ErrSubjectTokenRequired = errors.New("auth/oauth2client: subject token required")

	// ErrNoTokenEndpoint is returned by the *FromDiscovery constructors when the
	// [TokenEndpointSource] has not discovered a token endpoint URL.
	ErrNoTokenEndpoint = errors.New("auth/oauth2client: no token endpoint discovered")

	// ErrTokenRequest wraps a failed hand-rolled token request (the
	// client_credentials fetch used when a [ClientAuthenticator] is configured,
	// and the [JWTBearer] grant): a transport error or a non-2xx response. An
	// unparsable body, or one without access_token, from that request wraps
	// [ErrTokenExchange] instead, because the response parser is shared with
	// [Exchanger]. A non-2xx [JWTBearer] response also carries an
	// [*oauth2.RetrieveError] with the status and the IdP's error code.
	ErrTokenRequest = errors.New("auth/oauth2client: token request failed")

	// ErrRevocation wraps a failed token revocation ([Revoker.Revoke]): a missing
	// token, a transport error, or a non-2xx response from the revocation endpoint.
	ErrRevocation = errors.New("auth/oauth2client: token revocation failed")

	// ErrDeviceAuth wraps a failed device authorization grant ([DeviceFlow]): a
	// failed authorization request, or a token poll that ended in denial, expiry,
	// or a transport error. The underlying *oauth2.RetrieveError (with the RFC
	// 8628 error code) stays reachable via errors.As.
	ErrDeviceAuth = errors.New("auth/oauth2client: device authorization failed")

	// ErrInsecureEndpoint is returned when a request would go to an endpoint
	// that is not https (plain http is accepted on a loopback host only) and
	// the grant requires HTTPS: the client credentials would travel in the
	// clear. The flows report it from the request, wrapped like a transport
	// error; the *FromDiscovery constructors return it for a discovered endpoint.
	ErrInsecureEndpoint = errors.New("auth/oauth2client: insecure endpoint")

	// ErrRedirectNotAllowed is returned when an endpoint redirects a request
	// of an injected client that has its own CheckRedirect
	// to another origin: following it would re-send the request body, client
	// credentials included, to that origin. The flows report it wrapped like
	// a transport error, and it is never retried.
	ErrRedirectNotAllowed = errors.New("auth/oauth2client: redirect to another origin")
)
