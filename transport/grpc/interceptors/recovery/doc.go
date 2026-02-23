// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package recovery provides gRPC interceptors for recovering from panics in RPC handlers.
// It catches panics and converts them into proper gRPC errors, preventing server crashes.
//
// Features:
//   - Unary and streaming server interceptors
//   - Unary and streaming client interceptors
//   - Configurable panic handlers
//   - Stack trace capture with goroutine ID
//   - Request ID logging integration
//   - Method filtering via exact match or regex patterns
//
// The package uses shared recovery utilities from internal/recovery for consistent
// panic handling across HTTP and gRPC services.
//
// Example:
//
//	server := grpc.NewServer(
//	    grpc.UnaryInterceptor(recovery.ServerInterceptor().ServerUnaryInterceptor()),
//	)
//
//	// In your panicHandler - any panic will be caught
//	func (s *server) MyMethod(ctx context.Context, req *pb.Request) (*pb.Response, error) {
//	    if req.Id == "" {
//	        panic("ID is required") // Will be converted to gRPC error
//	    }
//	    return &pb.Response{}, nil
//	}
package recovery
