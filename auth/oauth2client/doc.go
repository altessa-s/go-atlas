// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package oauth2client acquires OAuth2 tokens from an external identity
// provider as a client. It complements the verification side
// ([github.com/altessa-s/go-atlas/auth/oidc]) and the self-issuing side
// ([github.com/altessa-s/go-atlas/auth/selfjwt]): those check and mint tokens,
// while this package fetches them for outbound, service-to-service calls.
//
// Every helper yields an [golang.org/x/oauth2.TokenSource] — the same currency
// consumed by
// [github.com/altessa-s/go-atlas/transport/grpc/client.NewInsecureTokenCredentials]
// and [github.com/altessa-s/go-atlas/auth/oidc.Provider.UserInfo] — so an
// acquired token drops straight into the transport layer with no adapter.
//
// # Grants
//
// The four x/oauth2-native grants are thin wrappers that add consistent
// grant-specific options and return a self-refreshing source:
//
//   - [ClientCredentials] — service authenticates as itself (RFC 6749 §4.4).
//   - [Refresh]           — resume a session from a stored refresh token (§6).
//   - [AuthCode]          — human consent flow (§4.1): build the URL, then
//     exchange the returned code.
//   - [DeviceFlow]        — device authorization grant (RFC 8628) for
//     input-constrained clients: request a code, then poll for the token.
//
// RFC 8693 token exchange has no x/oauth2 equivalent and is
// implemented here as a spec-compliant client:
//
//   - [Exchanger] — trade a subject token (optionally with an actor token, for
//     delegation) for a token scoped to a downstream audience or resource.
//
// The JWT bearer authorization grant (RFC 7523 §2.1), used by Google service
// accounts, is hand-rolled too, because x/oauth2/jwt does not bind the token
// request to the caller's context and accepts a token without expires_in:
//
//   - [JWTBearer] — sign an assertion (iss, aud = token URL, scope, iat, exp)
//     with an asymmetric [github.com/altessa-s/go-atlas/auth/jwt.SigningKey]
//     and trade it for an access token. [JWTBearer.Token] runs the exchange on
//     the caller's context, rejects a response without a positive expires_in,
//     caches the token until [WithJWTBearerEarlyExpiry] before its expiry, and serializes
//     refreshes so concurrent callers share one exchange.
//
// # Client authentication
//
// [WithClientCredentialsClientAuth] authenticates the client with a signed JWT assertion
// (RFC 7523) instead of a shared secret — [PrivateKeyJWT] for private_key_jwt
// and [ClientSecretJWT] for client_secret_jwt. It applies to [ClientCredentials]
// with corresponding options for [Exchanger] and [Revoker].
//
// # Refresh timing and revocation
//
// [WithClientCredentialsEarlyExpiry], [WithExchangerEarlyExpiry], and
// [WithJWTBearerEarlyExpiry] set the refresh margin for their respective grants.
// [Revoker] revokes an acquired token at the IdP revocation endpoint (RFC 7009).
// [Exchanger] and [JWTBearer] have built-in retry options. Other grants can
// use an HTTP client whose transport is wrapped with [RetryTransport].
//
// # Secrets
//
// Client secrets are passed as strings and held for the token source's lifetime
// (they are presented on every refresh), so they cannot be zeroized after use.
// Deployments that must avoid a long-lived in-memory secret should prefer
// [PrivateKeyJWT], where the client holds a private key the caller manages.
//
// # Error confidentiality
//
// The identity provider is untrusted: its status line, headers, Location,
// body, and trailers may echo secrets. Returned error text and package logs
// withhold IdP bodies, error descriptions, error URIs, decoded values, and
// IdP-supplied URLs, including a followed redirect URL when a later hop
// fails. They may show the numeric HTTP status, registered RFC 6749/8628
// error codes from the fixed allowlist, and body or withheld-field byte
// lengths.
//
// Before a response is parsed, error text is retained only for cancellation
// or deadline sentinels, all dial-phase [net.OpError] regardless of their inner
// cause, [net.DNSError], connection errnos ECONNREFUSED, ECONNRESET,
// EHOSTUNREACH, ENETUNREACH and ETIMEDOUT,
// [tls.CertificateVerificationError], [x509.CertificateInvalidError],
// [x509.HostnameError], [x509.UnknownAuthorityError], [x509.SystemRootsError],
// [tls.AlertError], [tls.RecordHeaderError], and package guard or grant sentinels.
// URL, syscall, DNS and certificate-verification wrappers qualify only when
// every nested cause qualifies. Read/write and proxyconnect net.OpError values
// remain summarized; proxy setup can read peer bytes, including CONNECT responses.
// Any other pre-response failure, including an unparseable IdP response or
// unknown wrapper, is summarized with its cause reachable through errors.As.
// This also applies to every [RetryTransport] attempt when used on its own.
//
// Application-supplied HTTP clients/transports, [ClientAuthenticator]
// implementations, discovery sources, and configured endpoint URLs are
// trusted; errors they produce on their own are outside this guarantee.
// Original causes, including response bodies, remain available through
// errors.Is/errors.As where preserved; avoid logging those causes directly.
//
// Some x/oauth2 body-read and JSON decoder errors are flattened upstream, so
// their original causes cannot be recovered through errors.Is/errors.As.
//
// # Observability
//
// Each grant has metrics and logger options. Metrics record fetch counts,
// latency and retries by grant and outcome. Logging records failures and
// retries. Revocation has no fetch metrics or fetch logger.
//
// # Usage
//
//	src, err := oauth2client.ClientCredentials(ctx, tokenURL, clientID, clientSecret,
//	    oauth2client.WithClientCredentialsScopes("orders:read", "orders:write"),
//	)
//	if err != nil {
//	    return err // tokenURL is malformed or not https
//	}
//	creds := client.NewInsecureTokenCredentials(src) // outbound gRPC calls now carry a fresh token
//
// JWT bearer grant for a service account, bounded by the call's deadline:
//
//	bearer, err := oauth2client.NewJWTBearer(tokenURL, clientEmail,
//	    jwt.SigningKey{KeyID: keyID, Algorithm: jwt.AlgRS256, Key: rsaKey},
//	    oauth2client.WithJWTBearerScopes("https://www.googleapis.com/auth/cloud-platform"),
//	    oauth2client.WithJWTBearerEarlyExpiry(5*time.Minute),
//	)
//	tok, err := bearer.Token(ctx)
//
// Token exchange, propagating the caller's identity to a downstream service:
//
//	ex, err := oauth2client.NewExchanger(tokenURL, clientID, clientSecret)
//	if err != nil {
//	    return err
//	}
//	tok, err := ex.Exchange(ctx, oauth2client.ExchangeRequest{
//	    SubjectToken: inboundAccessToken,
//	    Audience:     "https://downstream.internal",
//	})
package oauth2client
