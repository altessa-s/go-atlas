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
// The three x/oauth2-native grants are thin wrappers that add consistent
// [Option] configuration and return a self-refreshing source:
//
//   - [ClientCredentials] — service authenticates as itself (RFC 6749 §4.4).
//   - [Refresh]           — resume a session from a stored refresh token (§6).
//   - [AuthCode]          — human consent flow (§4.1): build the URL, then
//     exchange the returned code.
//   - [DeviceFlow]        — device authorization grant (RFC 8628) for
//     input-constrained clients: request a code, then poll for the token.
//
// The fourth, RFC 8693 token exchange, has no x/oauth2 equivalent and is
// implemented here as a spec-compliant client:
//
//   - [Exchanger] — trade a subject token (optionally with an actor token, for
//     delegation) for a token scoped to a downstream audience or resource.
//
// # Client authentication
//
// [WithClientAuth] authenticates the client with a signed JWT assertion
// (RFC 7523) instead of a shared secret — [PrivateKeyJWT] for private_key_jwt
// and [ClientSecretJWT] for client_secret_jwt. It applies to [ClientCredentials]
// and [Exchanger]; [Refresh] and [AuthCode] keep the secret path.
//
// # Refresh timing and revocation
//
// [WithEarlyExpiry] refreshes a token a configurable window before its exp to
// absorb clock skew and in-flight latency (applies to [ClientCredentials] and
// [Exchanger.TokenSource]). [Revoker] revokes an acquired token at the IdP
// revocation endpoint (RFC 7009), the symmetric counterpart to acquisition.
// Only [Exchanger] retries by itself; wrap the transport with [RetryTransport]
// and inject it via [WithHttpClient] to retry the x/oauth2-backed grants.
//
// # Secrets
//
// Client secrets are passed as strings and held for the token source's lifetime
// (they are presented on every refresh), so they cannot be zeroized after use.
// Deployments that must avoid a long-lived in-memory secret should prefer
// [PrivateKeyJWT], where the client holds a private key the caller manages.
//
// # Observability
//
// [WithMetrics] records fetch counts, latency, and retry counts (labeled by
// grant and outcome); cache hits are not counted. [WithLogger] logs fetch
// failures and retry attempts. Both default to off, keeping the hot path
// allocation-free.
//
// # Usage
//
//	src := oauth2client.ClientCredentials(ctx, tokenURL, clientID, clientSecret,
//	    oauth2client.WithScopes("orders:read", "orders:write"),
//	)
//	creds := client.NewInsecureTokenCredentials(src) // outbound gRPC calls now carry a fresh token
//
// Token exchange, propagating the caller's identity to a downstream service:
//
//	ex := oauth2client.NewExchanger(tokenURL, clientID, clientSecret)
//	tok, err := ex.Exchange(ctx, oauth2client.ExchangeRequest{
//	    SubjectToken: inboundAccessToken,
//	    Audience:     "https://downstream.internal",
//	})
package oauth2client
