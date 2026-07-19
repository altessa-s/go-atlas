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
// # Trust model
//
// A client-supplied metadata value that passes strict UUID v4 validation
// is trusted as-is: it is propagated, stored in the context, and used as
// a log correlation field, so a hostile client chooses which UUID
// appears in logs and can reuse one across requests to spoof
// correlation. Values that fail validation never propagate, so arbitrary
// client bytes cannot reach logs through this metadata key. There is no
// option to ignore a valid inbound value — strip or replace the metadata
// at the edge proxy when server-authoritative IDs are required.
//
// Example:
//
//	gen := requestid.NewGenerator()
//	server := grpc.NewServer(grpc.UnaryInterceptor(requestid.ServerUnaryInterceptor(gen)))
//	// In handler: reqID := requestid.FromContext(ctx)
package requestid
