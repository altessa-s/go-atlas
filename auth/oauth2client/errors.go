// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client

import "errors"

// Sentinel errors returned by [Exchanger]. Errors from [ClientCredentials],
// [Refresh], and [AuthCode] surface the underlying x/oauth2 error unchanged
// (typically an [*oauth2.RetrieveError]) so callers can inspect the IdP's
// status code and body directly.
var (
	// ErrTokenExchange wraps a failed RFC 8693 token exchange: a transport
	// error, a non-2xx response from the token endpoint, or an unparsable body.
	// The IdP's status and a truncated body are attached to the message.
	ErrTokenExchange = errors.New("auth/oauth2client: token exchange failed")

	// ErrSubjectTokenRequired is returned by [Exchanger.Exchange] when the
	// [ExchangeRequest] carries an empty SubjectToken. RFC 8693 requires it.
	ErrSubjectTokenRequired = errors.New("auth/oauth2client: subject token required")

	// ErrNoTokenEndpoint is returned by the *FromDiscovery constructors when the
	// [TokenEndpointSource] has not discovered a token endpoint URL.
	ErrNoTokenEndpoint = errors.New("auth/oauth2client: no token endpoint discovered")

	// ErrTokenRequest wraps a failed hand-rolled token request (the
	// client_credentials fetch used when a [ClientAuthenticator] is configured):
	// a transport error, a non-2xx response, or an unparsable body.
	ErrTokenRequest = errors.New("auth/oauth2client: token request failed")

	// ErrRevocation wraps a failed token revocation ([Revoker.Revoke]): a missing
	// token, a transport error, or a non-2xx response from the revocation endpoint.
	ErrRevocation = errors.New("auth/oauth2client: token revocation failed")

	// ErrDeviceAuth wraps a failed device authorization grant ([DeviceFlow]): a
	// failed authorization request, or a token poll that ended in denial, expiry,
	// or a transport error. The underlying *oauth2.RetrieveError (with the RFC
	// 8628 error code) stays reachable via errors.As.
	ErrDeviceAuth = errors.New("auth/oauth2client: device authorization failed")
)
