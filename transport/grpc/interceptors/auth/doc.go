// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package auth provides gRPC interceptors for token-based authentication and
// scope-based authorization.
//
// Use [ServerInterceptor] with an [Auth] function to validate tokens on the
// server side. For client-side token injection, use [NewPerRPCCredentials], whose
// credentials require transport security. Scope-based authorization is layered on via
// [ScopeClientAuth], which drives a transport-neutral
// [ScopeEnforcer] from the verified
// principal in [Credentials].
//
// Bearer tokens are extracted from the "authorization" gRPC metadata header.
// After successful authentication, [Credentials] are injected into the context
// — whether or not a [ClientAuth] is configured — and can be retrieved with
// [CredentialsFromContext]. When the authenticated data is a
// [github.com/altessa-s/go-atlas/auth/principal.Principal] (or a non-nil
// pointer to one), it is also stored through principal.NewContextFrom; data
// that maps itself onto a principal, such as the *oidc.Claims of the oidc
// adapter, is stored as its Principal(). A handler served over gRPC and HTTP
// reads the caller with principal.FromContext.
//
// Callback errors retain their gRPC code and errors.Is matches. Client statuses
// and diagnostic records contain only fixed classifications. A callback that
// refuses deliberately returns a [Rejection], whose well-formed AIP-193 reason
// and domain also reach the client as an ErrorInfo.
//
// No method is exempt from authentication by default, gRPC health and
// reflection included. Opt methods out with exact names via WithIgnoreMethods
// or with patterns anchored at both ends via WithIgnorePatterns; the
// reflection and health exemptions applied by default before are restored with
// WithIgnorePatterns(defaults.IgnorePatterns...).
//
// The oidc sub-package provides oidc.AuthFunc, a thin adapter that runs an
// [github.com/altessa-s/go-atlas/auth/oidc.ClaimsValidator] (typically
// auth/oidc.NewValidator over an auth/oidc.Provider) and maps failures to
// codes.Unauthenticated.
//
// Example:
//
//	interceptor := auth.ServerInterceptor(
//	    auth.WithAuthFn(authFunc),
//	    auth.WithIgnoreMethods("/grpc.health.v1.Health/Check"),
//	)
//	server := grpc.NewServer(grpc.UnaryInterceptor(interceptor.ServerUnaryInterceptor()))
package auth
