// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package grpc provides gRPC server and client utilities for go-atlas services.
//
// # Sub-packages
//
//   - [client] -- gRPC client with connection pooling, retry, and error handling.
//     Use [client.New] to create a client and [client.Client.GetConnection] to
//     obtain a [grpc.ClientConn].
//   - [handlers] -- service handlers (health checking, scheduler management)
//     that implement the [server.Handler] interface.
//   - [interceptors] -- middleware chain for logging, auth, metrics, recovery,
//     etc. Use [interceptors.NewChain] to assemble interceptors with automatic
//     dependency ordering.
//   - [server] -- gRPC server lifecycle with graceful shutdown and TLS.
//     Use [server.New] to create a server and [server.Server.Start] to begin
//     accepting connections.
//   - [server/factory] -- configuration-driven server and interceptor creation.
package grpc
