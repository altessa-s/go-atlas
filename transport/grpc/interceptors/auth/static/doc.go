// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package static plugs static-token authentication into the gRPC auth
// interceptor.
//
// The package is a thin adapter over
// [github.com/altessa-s/go-atlas/auth/static]. It provides [AuthFunc], which maps store errors to gRPC
// status codes ([codes.Unauthenticated], [codes.ResourceExhausted],
// [codes.Internal]). Token storage, hashing, and metrics live in
// auth/static and are shared with the HTTP middleware adapter.
//
// # Usage
//
// Import auth/static as staticstore for store construction.
//
//	store := staticstore.NewInMemoryStore(
//	    staticstore.WithInitialTokens(map[string]any{
//	        "sk_live_xxx": UserInfo{ID: "u1"},
//	    }),
//	)
//
//	interceptor := auth.ServerInterceptor(
//	    auth.WithAuthFn(static.AuthFunc(store)),
//	)
//
// # API key via custom header
//
//	extractor := auth.ExtractTokenFromHeader("x-api-key", func(v string) (string, error) {
//	    return v, nil
//	})
//
//	interceptor := auth.ServerInterceptor(
//	    auth.WithTokenExtractor(extractor),
//	    auth.WithAuthFn(static.AuthFunc(store)),
//	)
package static
