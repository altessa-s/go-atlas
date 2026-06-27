// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package auth provides gRPC interceptors for token-based authentication and
// scope-based authorization.
//
// Use [ServerInterceptor] with an [Auth] function to validate tokens on the
// server side. For client-side token injection, use [ClientInterceptor] with
// [TokenCredentials]. Scope-based authorization is layered on via
// [ScopeClientAuth], which drives a transport-neutral
// [github.com/altessa-s/go-atlas/auth/scope.Enforcer] from the verified
// principal in [Credentials].
//
// Bearer tokens are extracted from the "authorization" gRPC metadata header.
// After successful authentication, [Credentials] are injected into the context
// and can be retrieved with CredentialsFromContext.
//
// The [oidc] sub-package provides [oidc.AuthFunc] for OIDC/JWT validation,
// and [oidc/validator] provides a production-ready [validator.DefaultValidator].
//
// Example:
//
//	interceptor := auth.ServerInterceptor(
//	    auth.WithAuthFunc(authFunc),
//	    auth.WithIgnoreMethods("/grpc.health.v1.Health/Check"),
//	)
//	server := grpc.NewServer(grpc.UnaryInterceptor(interceptor.ServerUnaryInterceptor()))
package auth
