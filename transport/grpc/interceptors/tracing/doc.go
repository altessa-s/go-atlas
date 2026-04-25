// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package tracing provides gRPC interceptors for distributed tracing.
//
// The interceptors integrate with the observability/tracing package to provide:
//   - Automatic span creation for server and client calls
//   - W3C Trace Context propagation via gRPC metadata
//   - Span attributes for gRPC method, status code, and peer information
//   - Error recording with status mapping
//
// # Server Interceptor
//
// The server interceptor creates a span for each incoming RPC:
//
//	interceptor := tracing.ServerInterceptor(tracerProvider,
//	    tracing.WithIgnoreMethods("/grpc.health.v1.Health/Check"),
//	)
//	server := grpc.NewServer(
//	    grpc.ChainUnaryInterceptor(interceptor.ServerUnaryInterceptor()),
//	    grpc.ChainStreamInterceptor(interceptor.ServerStreamInterceptor()),
//	)
//
// # Client Interceptor
//
// The client interceptor creates a span for each outgoing RPC and propagates
// the trace context via gRPC metadata:
//
//	interceptor := tracing.ClientInterceptor(tracerProvider)
//	conn, err := grpc.Dial(target,
//	    grpc.WithChainUnaryInterceptor(interceptor.ClientUnaryInterceptor()),
//	    grpc.WithChainStreamInterceptor(interceptor.ClientStreamInterceptor()),
//	)
//
// # Span Attributes
//
// The following attributes are automatically added to spans:
//   - rpc.system: "grpc"
//   - rpc.service: Service name from full method
//   - rpc.method: Method name from full method
//   - rpc.grpc.status_code: gRPC status code
//   - net.peer.name: Peer address (if available)
//
// # Dependencies
//
// The tracing interceptor declares dependencies on "metadata" interceptor
// for proper ordering in interceptor chains.
package tracing
