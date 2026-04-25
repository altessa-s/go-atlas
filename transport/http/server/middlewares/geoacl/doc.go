// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package geoacl provides HTTP middleware for geographic access control.
// It enforces allow and deny rules per URL path based on the geographic
// location of the client IP, resolved via a [geoacl.GeoResolver].
//
// Use [New] for the full [middlewares.Middleware] interface, or [Middleware]
// for a convenience wrapper returning func(http.Handler) http.Handler.
// The middleware declares a dependency on the "realip" middleware so that
// the client IP is always available in the request context.
//
// Denied requests receive HTTP 403 Forbidden. When no valid client IP is
// available or the geo resolver returns an error, the configured fallback
// behavior (default: deny) determines the outcome.
//
// Example:
//
//	registry := geoacl.NewRegistry(geoacl.PolicyDeny)
//	registry.SetDefault(&geoacl.AccessRule{
//	    AllowContinents: []string{"EU", "NA"},
//	})
//
//	mw := geoacl.New(resolver, registry, geoacl.WithFallbackBehavior(fallback.Deny))
//	handler := mw.Handler(myHandler)
package geoacl
