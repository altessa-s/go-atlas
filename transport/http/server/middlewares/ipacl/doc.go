// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package ipacl provides HTTP middleware for IP-based access control.
// It enforces allowlist and denylist rules per URL path, blocking or
// permitting requests based on the client IP resolved by the "realip"
// middleware.
//
// Use [New] for the full [middlewares.Middleware] interface, or [Middleware]
// for a convenience wrapper returning func(http.Handler) http.Handler.
// The middleware declares a dependency on the "realip" middleware so that
// the client IP is always available in the request context.
//
// Denied requests receive HTTP 403 Forbidden. When no valid client IP is
// available, the configured fallback behavior (default: deny) determines
// the outcome.
//
// Example:
//
//	registry := ipacl.NewRegistry(ipacl.PolicyDeny)
//	registry.SetDefault(&ipacl.AccessRule{
//	    Allowlist: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")},
//	})
//
//	mw := ipacl.New(registry, ipacl.WithFallbackBehavior(fallback.Deny))
//	handler := mw.Handler(myHandler)
package ipacl
