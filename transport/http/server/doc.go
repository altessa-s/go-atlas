// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package server provides an HTTP server with graceful shutdown, middleware support,
// and pluggable router implementations.
//
// The server abstracts the router implementation behind a [router.Router] interface,
// allowing different routing libraries to be used interchangeably. Handlers receive
// a [writer.ReadWriter] that provides content-negotiated request/response handling
// instead of raw http.ResponseWriter and *http.Request.
//
// # Lifecycle
//
// Routes and middleware must be registered before [Server.Start]. The server
// applies an error-interceptor middleware first, then user middleware, then
// registers handlers. If the router implements [InitializableRouter], its
// Initialize method is called before the listener is opened.
//
// [Server.Shutdown] signals the stop channel, closes the listener, and drains
// in-flight requests within the provided context deadline.
//
// # Subpackages
//
//   - codec: Content-type codecs and registry with content negotiation
//   - factory: Configuration-driven server and middleware creation
//   - handler: Common HTTP handlers (ping, healthz, pprof, metrics)
//   - middlewares: Middleware interfaces, chain, and dependency-based ordering
//   - responder: Structured error response interception
//   - router: Router interfaces and implementations (gorilla, std)
//   - writer: Content-negotiated response writing with builder pattern
//
// # Example
//
//	import (
//	    "github.com/altessa-s/go-atlas/transport/http/server"
//	    "github.com/altessa-s/go-atlas/transport/http/server/router/gorilla"
//	    baseserver "github.com/altessa-s/go-atlas/transport/internal/server"
//	)
//
//	srv, err := server.New(
//	    server.WithRouter(gorilla.New()),
//	    server.WithBaseOptions(baseserver.WithAddress(":8080")),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	srv.RegisterMiddleware(middleware...)
//	srv.RegisterHandlers(handlers...)
//	srv.Start()
//	defer srv.Shutdown(ctx)
//
// After Start succeeds, wait on Done to observe a post-start Serve failure or
// completion of lifecycle shutdown, including cancellation after the lifecycle
// claim. Protocol cleanup may finish after Done closes on cancellation.
// Err returns the first post-start Serve error; it remains nil on normal shutdown.
// Early Start errors are returned synchronously and leave Done open. Retrying is
// meaningful for listen/bind or pre-Serve errors. Auto-created listeners are
// cleared after early startup errors; a WithListener listener may be consumed by
// a failed Serve and is never replaced.
//
//	if err := srv.Start(); err != nil {
//	    return err // Do not wait on Done for an early startup failure.
//	}
//	<-srv.Done()
//	return srv.Err()
package server
