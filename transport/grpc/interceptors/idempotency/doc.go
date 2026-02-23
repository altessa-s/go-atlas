// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package idempotency provides gRPC interceptors for idempotent request handling.
// It prevents duplicate processing of requests with the same idempotency key
// using the driven interceptor pattern.
//
// Use [ServerInterceptor] to create the interceptor. The idempotency key is
// extracted from gRPC metadata (configurable via WithIdempotencyKeyHeader)
// and looked up in the provided [idempotencydata.Idempotency] storage.
// Duplicate requests receive the previously stored response without
// re-executing the handler.
//
// The [ErrorScenario] constants define possible error outcomes for
// idempotency key processing.
//
// Example:
//
//	storage := &redisStorage{client: redisClient, ttl: 24 * time.Hour}
//	interceptor := idempotency.ServerInterceptor(storage,
//	    idempotency.WithIdempotencyKeyHeader("x-idempotency-key"),
//	)
//	server := grpc.NewServer(
//	    grpc.UnaryInterceptor(interceptor.ServerUnaryInterceptor()),
//	)
package idempotency
