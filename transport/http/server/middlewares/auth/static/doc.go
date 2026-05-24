// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package static plugs static-token authentication into the HTTP auth
// middleware.
//
// The package is a thin adapter over
// [github.com/altessa-s/go-atlas/auth/static]: it re-exports the store
// types/options and provides [AuthFunc], which wraps store errors with the
// transport-level [auth.ErrUnauthorized] sentinel while preserving the
// original cause for logs and custom [auth.ErrorHandler] implementations.
//
// Token storage, hashing, and metrics live in auth/static and are shared
// with the gRPC interceptor adapter.
//
// # Usage
//
//	store := static.NewInMemoryStore(
//	    static.WithInitialTokens(map[string]any{
//	        "sk_live_xxx": UserInfo{ID: "u1"},
//	    }),
//	)
//
//	middleware := auth.Middleware(
//	    auth.WithAuthFunc(static.AuthFunc(store)),
//	)
//	handler := middleware(myHandler)
//
// # API key via custom header
//
//	extractor := auth.ExtractTokenFromHeader("X-API-Key", func(v string) (string, error) {
//	    return v, nil
//	})
//
//	middleware := auth.Middleware(
//	    auth.WithTokenExtractor(extractor),
//	    auth.WithAuthFunc(static.AuthFunc(store)),
//	)
package static
