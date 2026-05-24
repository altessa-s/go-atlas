// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package auth provides HTTP authentication middleware with support for
// various token schemes including Bearer tokens and API keys.
//
// The middleware is highly configurable and supports:
//   - Multiple token extraction methods (Bearer, API keys, custom headers)
//   - Pluggable authentication functions
//   - Path-based filtering to skip authentication
//   - Custom error handling
//   - Context-based auth data propagation
//
// # Basic Usage
//
// With Bearer tokens:
//
//	middleware := auth.Middleware(
//	    auth.WithAuthFunc(myAuthFunc),
//	)
//
//	handler := middleware(myHandler)
//
// With API keys:
//
//	extractor := auth.ExtractTokenFromHeader("X-API-Key", func(v string) (string, error) {
//	    return v, nil
//	})
//
//	middleware := auth.Middleware(
//	    auth.WithTokenExtractor(extractor),
//	    auth.WithAuthFunc(myAuthFunc),
//	)
//
// # Authentication Flow
//
// 1. Token extraction from the request (via TokenExtractor)
// 2. Token validation (via AuthFunc)
// 3. Auth data storage in context
// 4. Request forwarding to next handler
//
// # Error Handling
//
// The middleware provides default error responses:
//   - Missing token: 401 with WWW-Authenticate header
//   - Invalid token: 401 with error message
//
// Custom error handling can be configured:
//
//	middleware := auth.Middleware(
//	    auth.WithErrorHandler(customErrorHandler),
//	)
//
// # Context Usage
//
// Authentication data is stored in the request context and can be retrieved:
//
//	func handler(w http.ResponseWriter, r *http.Request) {
//	    authData := auth.FromContext(r.Context())
//	    if userInfo, ok := authData.(UserInfo); ok {
//	        // Use authenticated user info
//	    }
//	}
//
// # Path Filtering
//
// Skip authentication for specific paths:
//
//	middleware := auth.Middleware(
//	    auth.WithIgnorePaths([]string{"/health", "/metrics"}),
//	    auth.WithAuthFunc(myAuthFunc),
//	)
//
// # Subpackages
//
// The static subpackage provides pre-built authentication for static tokens:
//
//	import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth/static"
//
//	store := static.NewInMemoryStore(...)
//	authFunc := static.AuthFunc(store)
//
//	middleware := auth.Middleware(
//	    auth.WithAuthFunc(authFunc),
//	)
package auth
