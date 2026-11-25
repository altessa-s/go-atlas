// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package oidc provides OpenID Connect JWT token validation with automatic JWKS key rotation.
// It supports signature verification, claims validation, token introspection (RFC 7662),
// validation presets with matchers (native and CEL-based), and userinfo retrieval.
//
// Example:
//
//	provider, _ := oidc.NewProvider(ctx,
//	    "https://accounts.google.com/.well-known/openid-configuration",
//	    oidc.WithHTTPClient(httpclient.New()),
//	    oidc.WithDefaultValidationOptions(
//	        oidc.WithValidationIssuer("https://accounts.google.com"),
//	        oidc.WithValidationAudience("my-client-id"),
//	    ),
//	)
//	defer provider.Close()
//	claims, _ := provider.ValidateToken(ctx, token)
//
// JSON service policies are loaded by factory/auth/oidc.LoadServiceConfigOptions
// and passed to NewProvider as resolved options. A validation policy can bound
// the exact exp - iat interval with [WithValidationMinTokenLifetime] and
// [WithValidationMaxTokenLifetime]. Both bounds apply on signature-cache hits.
//
// # Typed claims
//
// [NewValidator] wraps a [TokenVerifier] (typically the [*Provider]) and
// projects the verified claim set onto [Claims]. It returns plain, wrapped
// errors so [errors.Is] still matches [ErrTokenInvalid] and [ErrTokenRevoked];
// the HTTP and gRPC auth adapters consume it through [ClaimsValidator] and map
// failures to 401 and codes.Unauthenticated respectively.
//
//	v := oidc.NewValidator(provider)
//	c, err := v.ValidateToken(ctx, token) // c is *oidc.Claims
//
// # Outbound policy
//
// Redirects stay on the original HTTPS origin regardless of discovery validation
// mode. WithDiscoveryAllowLoopbackHTTP permits same-origin HTTP loopback redirects.
// JWKS fetches are bounded to 1 MiB and 100 keys; failed refreshes retain old keys.
//
// A verification key must permit signing use and, when key_ops is present,
// include verify. Signed UserInfo responses enforce the JWKS freshness policy.
// Introspection responses require an explicit boolean active before caching.
//
// # Lifecycle
//
// The context passed to NewProvider bounds construction only. The refresh cron
// (WithJWKSRefreshSchedule, WithRevocationSyncSchedule) keeps its values but
// outlives its cancellation, so every provider must be closed: only
// [Provider.Close] stops and joins revocation recovery, releases the health
// registration and runs any dedicated filter cleanup.
//
// # Revocation freshness
//
// A provider that syncs its revocation storage on WithRevocationSyncSchedule
// rejects a "not revoked" answer with ErrRevocationCheck wrapping
// ErrRevocationStale once no sync succeeded within WithRevocationMaxStaleness
// (DefaultRevocationMaxStaleness; zero or negative disables the bound), unless
// WithRevocationFailOpen is set. Replicas sharing a Redis filter count the
// last rebuild committed by any of them (RevocationSyncReporter), read after
// each scheduled sync; a tick skipped because a peer is rebuilding does not
// count by itself. A scheduled sync is limited to
// WithRevocationSyncTimeout, and a sync that fails — an oversized or empty
// list included — keeps the current revocation set. WithRevocationAllowEmpty
// makes an empty list authoritative instead, for a source that can
// legitimately become empty.
//
// # Token cache trust
//
// A WithTokenCache hit reuses signature-verified claims without re-verifying the
// signature, so write access to the Cacher is write access to identities. Use a
// cache only this service can write. Signature entries preserve registered dates as
// exact JSON numbers. CEL expressions see the same dates as doubles.
package oidc
