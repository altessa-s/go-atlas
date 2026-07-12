// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package requestid provides gRPC interceptors for request ID generation and propagation.
// Ensures every request has a unique UUID v4 identifier for distributed tracing.
//
// Use [ServerInterceptor] (or the standalone [ServerUnaryInterceptor] /
// [ServerStreamInterceptor]) for server-side extraction and validation, and
// [ClientInterceptor] (or [ClientUnaryInterceptor] / [ClientStreamInterceptor])
// to propagate IDs to outgoing calls. Use [FromContext] and [NewContext] to
// read and inject request IDs in application code.
//
// When the generator is not configured with GenerateIfMissing and the incoming
// ID is absent or not a valid UUID v4, the server interceptor returns
// [ErrInvalidRequestId] as a gRPC [codes.InvalidArgument] status.
//
// Example:
//
//	gen := requestid.NewGenerator()
//	server := grpc.NewServer(grpc.UnaryInterceptor(requestid.ServerUnaryInterceptor(gen)))
//	// In handler: reqID := requestid.FromContext(ctx)
package requestid
