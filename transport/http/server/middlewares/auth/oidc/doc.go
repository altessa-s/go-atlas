// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package oidc plugs OIDC (OpenID Connect) token validation into the HTTP auth
// middleware.
//
// The package is a thin adapter over
// [github.com/altessa-s/go-atlas/auth/oidc]: token verification
// (oidc.Provider), the claims model (oidc.Claims) and claim extraction
// (oidc.Validator) live there and are shared with the gRPC interceptor
// adapter. [AuthFunc] runs an oidc.ClaimsValidator against the bearer token,
// stores the verified *oidc.Claims as the request's authenticated data, and
// wraps every failure with [auth.ErrUnauthorized] so the middleware answers
// 401.
//
// # Usage
//
//	provider, err := authoidc.NewProvider(ctx, discoveryURL,
//	    authoidc.WithHTTPClient(httpclient.New()),
//	)
//	if err != nil {
//	    return err
//	}
//
//	middleware := auth.Middleware(
//	    auth.WithAuthFunc(httpoidc.AuthFunc(authoidc.NewValidator(provider))),
//	)
//	handler := middleware(myHandler)
//
// A handler reads the claims with auth.FromContext:
//
//	claims, ok := auth.FromContext(r.Context()).(*authoidc.Claims)
//
// The middleware also stores the caller as the canonical principal mapped by
// oidc.Claims.Principal (default "tenant" and "roles" claims), so
// transport-neutral code reads it the same way as for any other
// authenticator:
//
//	p, ok := principal.FromContext(r.Context())
package oidc
