// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package gorilla provides a [router.Router] implementation using gorilla/mux.
//
// # Example
//
//	import (
//	    "github.com/altessa-s/go-atlas/transport/http/server"
//	    "github.com/altessa-s/go-atlas/transport/http/server/router/gorilla"
//	)
//
//	srv, err := server.New(
//	    server.WithRouter(gorilla.New()),
//	)
//
// # Accessing Underlying Router
//
// For advanced use cases requiring direct access to gorilla/mux features:
//
//	r := gorilla.New()
//	r.Underlying().StrictSlash(true)
//	srv, err := server.New(server.WithRouter(r))
package gorilla
