// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package router defines the [Router] and [Route] interfaces that abstract
// HTTP routing functionality.
//
// These interfaces decouple the server from any specific routing library,
// allowing implementations to be swapped without changing handler code.
// A [Router] supports handler and middleware registration, path-prefix
// matching, and subrouter creation.
//
// # Available Implementations
//
//   - gorilla: Uses gorilla/mux (feature-rich, regex patterns)
//   - std: Uses Go 1.22+ [http.ServeMux] (no external dependencies)
//
// # Example
//
//	import (
//	    "github.com/altessa-s/go-atlas/transport/http/server/router"
//	    "github.com/altessa-s/go-atlas/transport/http/server/router/gorilla"
//	)
//
//	var r router.Router = gorilla.New()
//	r.HandleFunc("/hello", helloHandler)
package router
