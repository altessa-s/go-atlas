// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package middlewares provides HTTP middleware interfaces, utilities, and
// dependency-based ordering.
//
// All chainable middlewares implement the [Middleware] interface which
// provides Name() for identification and Handler() for wrapping an
// [http.Handler]. Middlewares that implement [depgraph.DependencyDeclarer]
// participate in topological ordering via [OrderMiddlewares].
//
// [BaseMiddleware] is an embeddable struct providing path filtering,
// structured logging helpers, and response-writer wrapping. Most concrete
// middleware sub-packages embed it.
//
// [Chain] collects middlewares and applies them to a final handler.
// It supports dependency ordering, deduplication, and clone/extend
// operations.
//
// [MiddlewareError] carries an HTTP status code, user-facing message, and
// machine-readable error code for structured error responses from middleware.
//
// # Middleware Identity
//
// Every middleware sub-package exports three identification helpers:
//
//   - Name() string -- returns the middleware name as a plain string.
//   - ID -- a package-level [Middleware] variable (backed by [Noop]) for
//     typed exclusion lists (e.g., [factory.ServerBuilder.WithMiddlewares]).
//   - Dependencies / RequiredDependencies -- use sibling Name() functions
//     instead of string literals for compile-time safety.
//
// # Usage
//
//	mw := middlewares.Func("logging", func(next http.Handler) http.Handler {
//	    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//	        log.Printf("Request: %s", r.URL.Path)
//	        next.ServeHTTP(w, r)
//	    })
//	})
//
//	// Conditional middleware based on runtime state
//	auth := middlewares.ConditionalMiddleware(
//	    middlewares.MatchFunc(func() bool { return config.AuthEnabled }),
//	    authMiddleware,
//	)
package middlewares
