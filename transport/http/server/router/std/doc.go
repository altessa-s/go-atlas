// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package std provides a [router.Router] implementation using the standard
// library [http.ServeMux] with Go 1.22+ enhanced pattern matching.
//
// The router is sealed on the first call to [Router.ServeHTTP] or
// [Router.Initialize]. After sealing, any attempt to register routes or
// middleware panics. This design enables a single initialization of the
// middleware chain and route table, avoiding synchronization on the hot path.
//
// # Subrouters
//
// A subrouter shares its parent's mux and registers its routes under its full
// path prefix, so subrouters of one prefix coexist. Root middleware wraps every
// request; a subrouter's middleware wraps only its own routes.
//
// # Usage
//
//	r := std.New()
//	r.Handle("GET /api/users/{id}", userHandler)
//	r.PathPrefix("/static/").Handler(http.FileServer(http.Dir("./static")))
//	r.Use(loggingMiddleware, authMiddleware)
//	http.ListenAndServe(":8080", r)
package std
