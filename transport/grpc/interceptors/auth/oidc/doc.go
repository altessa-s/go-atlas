// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package oidc plugs OIDC (OpenID Connect) token validation into the gRPC auth
// interceptor.
//
// The package is a thin adapter over
// [github.com/altessa-s/go-atlas/auth/oidc]: token verification lives in
// oidc.Provider, and the transport-neutral claims model ([oidc.Claims]) and
// claim extraction ([oidc.Validator]) live there too, shared with the HTTP
// middleware adapter. [AuthFunc] only runs an [oidc.ClaimsValidator] against
// the request's bearer token and maps every validation failure to a
// codes.Unauthenticated status.
//
// # Basic Usage
//
//	import (
//	    authoidc "github.com/altessa-s/go-atlas/auth/oidc"
//	    "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
//	    grpcoidc "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth/oidc"
//	)
//
//	provider, err := authoidc.NewProvider(ctx,
//	    "https://auth.example.com/realms/myrealm/.well-known/openid-configuration",
//	    authoidc.WithHTTPClient(httpclient.New()),
//	    authoidc.WithDefaultValidationOptions(
//	        authoidc.WithValidationIssuer("https://auth.example.com/realms/myrealm"),
//	        authoidc.WithValidationAudience("my-service"),
//	    ),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer provider.Close()
//
//	authFunc := grpcoidc.AuthFunc(authoidc.NewValidator(provider))
//
//	server := grpc.NewServer(
//	    grpc.UnaryInterceptor(auth.ServerUnaryInterceptor(
//	        auth.WithAuthFn(authFunc),
//	    )),
//	)
//
// # Accessing Claims in Handlers
//
// After successful authentication, the verified *oidc.Claims are the
// credentials' Data:
//
//	func (s *MyService) GetUser(ctx context.Context, req *GetUserRequest) (*User, error) {
//	    creds, ok := auth.CredentialsFromContext(ctx)
//	    if !ok {
//	        return nil, status.Error(codes.Unauthenticated, "no credentials")
//	    }
//	    claims, ok := creds.Data.(*authoidc.Claims)
//	    if !ok {
//	        return nil, status.Error(codes.Unauthenticated, "unexpected credentials type")
//	    }
//	    return s.getUserBySubject(claims.Subject)
//	}
//
// The interceptor also stores the caller as the canonical principal mapped by
// oidc.Claims.Principal (default "tenant" and "roles" claims), so
// transport-neutral code reads it the same way as for any other
// authenticator:
//
//	p, ok := principal.FromContext(ctx)
//
// # Custom Validators
//
// Any [oidc.ClaimsValidator] works, for example a decorator adding checks
// against the raw claim set:
//
//	type tenantValidator struct{ inner authoidc.ClaimsValidator }
//
//	func (v tenantValidator) ValidateToken(ctx context.Context, token string) (*authoidc.Claims, error) {
//	    claims, err := v.inner.ValidateToken(ctx, token)
//	    if err != nil {
//	        return nil, err
//	    }
//	    if claims.RawClaims["tenant_id"] != "expected-tenant" {
//	        return nil, errors.New("invalid tenant")
//	    }
//	    return claims, nil
//	}
//
// # Integration with scope-based authorization
//
// Keep the AuthFunc focused on authentication and layer authorization on with
// github.com/altessa-s/go-atlas/auth/scope, wired through the auth
// interceptor's ScopeClientAuth:
//
//	registry := scope.NewRegistry()
//	registry.Register("/orders.v1.OrderService/GetOrder", "orders:read")
//	registry.Register("/orders.v1.OrderService/Ping", "")
//	registry.Freeze()
//
//	enf := scope.NewEnforcer(registry, scope.ScopeAuthorizer(authoidc.ScopesOf, scope.Exact()))
//
//	interceptor := auth.ServerInterceptor(
//	    auth.WithAuthFn(authFunc),
//	    auth.WithClientAuth(auth.ScopeClientAuth(enf)),
//	)
//
// Pass [WithAudit] to record each token-validation decision through an
// auth.DecisionRecorder; the audit reason distinguishes revoked and invalid
// tokens because the validator keeps the provider's error chain.
package oidc
