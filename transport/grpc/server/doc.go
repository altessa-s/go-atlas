// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package grpc provides a production-ready gRPC server wrapper with graceful
// shutdown, TLS support, and reflection.
//
// Create a [Server] with [New], register services via [Server.RegisterHandlers],
// optionally add interceptors with [Server.RegisterInterceptors], then call
// [Server.Start]. The [GRPCServer] interface defines the full contract.
//
// All exported methods on [Server] are safe for concurrent use after the server
// has started.
//
// # Features
//
//   - Lifecycle Management: graceful shutdown with configurable timeouts
//     via [Server.Shutdown].
//   - Security: integrated TLS support with automated certificate management.
//   - Observability: built-in support for logging, metrics (Prometheus), and tracing.
//   - Extensibility: easy registration of custom services via [Handler] and
//     interceptors via [interceptors.ServerInterceptor].
//
// # Usage
//
//	srv, _ := grpc.New(
//	    grpc.WithBaseOptions(
//	        server.WithAddress(":8080"),
//	    ),
//	    grpc.WithReflection(true),
//	)
//
//	srv.RegisterHandlers(myHandler)
//
//	if err := srv.Start(); err != nil {
//	    log.Fatal(err)
//	}
//	defer srv.Shutdown(ctx)
package grpc
