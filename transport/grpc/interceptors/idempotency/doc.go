// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package idempotency provides gRPC interceptors and helpers for idempotent
// request handling on both ends of the wire. The server-side interceptor
// dedupes mutating calls keyed by [DefaultIdempotencyKeyHeader]; the client
// side mints and attaches the same key from a stable operation seed so retries
// of one logical operation collapse on the server.
//
// # Server side
//
// Use [ServerInterceptor] to create the interceptor. The idempotency key is
// extracted from gRPC metadata (configurable via [WithIdempotencyKeyHeader]),
// or from a request field such as request_id ([WithKeyField]), checked
// by a [KeyFormatValidator] (UUID v4 by default, [OpaqueKeyValidator] or
// [PatternKeyValidator] for client request IDs), and looked up in the provided
// [idempotencydata.Idempotency] storage. A [MethodClassifier] (for example
// [VerbMethodClassifier]) decides per method whether the key is skipped,
// optional or mandatory.
// Duplicate requests receive the previously stored response without
// re-executing the handler. The [ErrorScenario] constants define possible
// error outcomes for idempotency key processing.
//
// # Client side
//
// Three helpers cover ad-hoc usage: [DeriveKey] produces a deterministic UUID
// v4 from a (seed, call) pair, [WithKey] attaches a caller-supplied key to the
// outgoing metadata, and [WithDerivedKey] composes the two.
//
// For systematic wiring, [UnaryClientInterceptor] auto-stamps a derived key on
// every outbound call whose context was tagged with [WithOperation]. An
// explicit [WithKey] / [WithDerivedKey] attachment on the same context wins
// over the seed-based path. Configure via [WithClientIdempotencyKeyHeader],
// [WithClientSeedExtractor], and [WithClientMethodFilter].
//
// # Examples
//
// Server:
//
//	storage := &redisStorage{client: redisClient, ttl: 24 * time.Hour}
//	interceptor := idempotency.ServerInterceptor(storage,
//	    idempotency.WithIdempotencyKeyHeader("x-idempotency-key"),
//	)
//	server := grpc.NewServer(
//	    grpc.UnaryInterceptor(interceptor.ServerUnaryInterceptor()),
//	)
//
// Client:
//
//	conn, err := grpc.NewClient(target,
//	    grpc.WithTransportCredentials(creds),
//	    grpc.WithUnaryInterceptor(idempotency.UnaryClientInterceptor()),
//	)
//	// ...
//	ctx = idempotency.WithOperation(ctx, operationID)
//	resp, err := client.Update(ctx, req)
package idempotency
