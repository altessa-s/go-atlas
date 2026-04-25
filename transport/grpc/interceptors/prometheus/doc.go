// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package prometheus provides comprehensive Prometheus metrics collection for gRPC servers.
// It implements both unary and streaming interceptors that automatically collect detailed
// metrics about request counts, durations, status codes, and optionally message sizes.
//
// The interceptor collects the following metrics by default:
//   - grpc_server_requests_total: Total request count with method and status labels
//   - grpc_server_requests_by_status_total: Request counts partitioned by status codes
//   - grpc_server_requests_by_method_total: Request counts partitioned by methods
//   - grpc_server_request_duration_seconds: Request latency histogram with method and status labels
//   - grpc_server_requests_in_flight: Current number of concurrent requests (gauge)
//
// Optional size metrics (enabled via WithEnableSizeMetrics):
//   - grpc_server_request_size_bytes: Request message size histogram
//   - grpc_server_response_size_bytes: Response message size histogram
//
// The interceptor is thread-safe and designed for production use with configurable
// metric namespaces, custom buckets, method ignoring, and flexible Prometheus registerer support.
//
// Example:
//
//	// Basic usage with default metrics
//	interceptor := prometheus.ServerInterceptor()
//
//	// Advanced configuration
//	interceptor := prometheus.ServerInterceptor(
//		prometheus.WithNamespace("myapp"),
//		prometheus.WithSubsystem("api"),
//		prometheus.WithEnableSizeMetrics(true),
//		prometheus.WithIgnoreMethods("/grpc.health.v1.Health/Check"),
//		prometheus.WithLogger(logger),
//	)
//
//	// Use with gRPC server
//	server := grpc.NewServer(
//		grpc.UnaryInterceptor(interceptor.ServerUnaryInterceptor()),
//		grpc.StreamInterceptor(interceptor.ServerStreamInterceptor()),
//	)
package prometheus
