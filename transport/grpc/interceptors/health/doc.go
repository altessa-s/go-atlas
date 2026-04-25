// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package health provides gRPC interceptors for service health checking.
// It ensures incoming requests are only processed when the service is healthy.
//
// Implement the [Health] interface and pass it to [ServerInterceptor] (or the
// standalone [ServerUnaryInterceptor] / [ServerStreamInterceptor] helpers).
// When a health check fails, the interceptor rejects the request with
// [codes.Unavailable] and wraps the underlying error with
// [ErrServiceUnavailable].
//
// Example:
//
//	type serviceHealth struct {
//	    db *sql.DB
//	}
//
//	func (h *serviceHealth) Health(ctx context.Context) error {
//	    return h.db.PingContext(ctx)
//	}
//
//	healthChecker := &serviceHealth{db: db}
//	server := grpc.NewServer(
//	    grpc.UnaryInterceptor(health.ServerUnaryInterceptor(healthChecker)),
//	)
package health
