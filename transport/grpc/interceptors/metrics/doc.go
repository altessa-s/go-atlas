// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package metrics provides comprehensive metrics collection for gRPC servers
// via the [observability/metrics.Collector] abstraction. It implements both
// unary and streaming interceptors that automatically collect detailed
// metrics about request counts, durations, status codes, and optionally
// message sizes.
//
// The interceptor collects the following metrics by default (names shown
// after [Collector.WithSubsystem] is applied; default subsystem is "grpc"):
//
//   - {service}_grpc_server_requests_total — total request count, labels: method, status
//   - {service}_grpc_server_request_duration_seconds — request latency histogram, labels: method, status
//   - {service}_grpc_server_requests_in_flight — current concurrent requests (gauge)
//   - {service}_grpc_server_requests_in_flight_by_method — concurrent requests partitioned by method (gauge)
//
// Optional size metrics (enabled via [WithEnableSizeMetrics]):
//
//   - {service}_grpc_server_request_size_bytes — request message size histogram
//   - {service}_grpc_server_response_size_bytes — response message size histogram
//
// Optional streaming metrics (enabled via [WithEnableStreamMetrics]):
//
//   - {service}_grpc_server_stream_messages_sent_total
//   - {service}_grpc_server_stream_messages_received_total
//   - {service}_grpc_server_stream_message_size_bytes (when both stream and size metrics are enabled)
//
// The interceptor is thread-safe and designed for production use with
// configurable metric subsystems, custom histogram buckets, method ignoring,
// and pluggable [metrics.Collector] backends. Each call to [ServerInterceptor]
// creates a fresh interceptor; the underlying adapter deduplicates metric
// registrations by name, so reusing the same collector across multiple calls
// with the same subsystem shares the same metric vectors.
//
// Example:
//
//	// Basic usage with default metrics.
//	interceptor := metrics.ServerInterceptor(metrics.WithCollector(coll))
//
//	// Advanced configuration with a custom subsystem and ignored methods.
//	interceptor := metrics.ServerInterceptor(
//		metrics.WithCollector(coll),
//		metrics.WithMetricsSubsystem("api"),
//		metrics.WithEnableSizeMetrics(),
//		metrics.WithIgnoreMethods("/grpc.health.v1.Health/Check"),
//		metrics.WithLogger(logger),
//	)
//
//	// Use with a gRPC server.
//	server := grpc.NewServer(
//		grpc.UnaryInterceptor(interceptor.ServerUnaryInterceptor()),
//		grpc.StreamInterceptor(interceptor.ServerStreamInterceptor()),
//	)
package metrics
