// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package interceptors provides a unified framework for gRPC interceptors.
//
// The core abstractions are:
//
//   - [Chain] -- collects interceptors and produces ordered [grpc.ServerOption]
//     or [grpc.DialOption] slices, automatically resolving dependency order via
//     topological sort.
//   - [BaseInterceptor] -- embeddable base with endpoint filtering, logging,
//     and name identification for custom interceptors.
//   - [ServerInterceptor] / [ClientInterceptor] -- interfaces for server-side
//     and client-side interception of both unary and streaming RPCs.
//   - [Error] -- combines a gRPC status with a standard Go error, implementing
//     both the error and status.GRPCStatus interfaces.
//
// The Driven Interceptor pattern (via [ServerDrivenInterceptor] and
// [ClientDrivenInterceptor]) decouples business logic from gRPC-specific
// function signatures by delegating to a [driver.Driver] with PreCall/PostCall
// hooks.
//
// [Chain] automatically prepends a metadata interceptor so that all subsequent
// interceptors receive pre-parsed [metadata.CallMetadata] in the context.
//
// Example:
//
//	chain := interceptors.NewChain(
//	    requestid.ServerInterceptor(),
//	    logger.ServerInterceptor(slog.Default()),
//	    recovery.ServerInterceptor(),
//	)
//	opts, err := chain.ServerOptions()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	server := grpc.NewServer(opts...)
package interceptors
