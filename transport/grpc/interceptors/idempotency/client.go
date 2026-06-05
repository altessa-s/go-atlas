// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"bytes"
	"context"
	"crypto/sha256"

	"github.com/google/uuid"

	"google.golang.org/grpc/metadata"
)

// keyNamespace anchors derived keys to this package so that identical (seed, call)
// inputs produced elsewhere cannot collide with keys minted here.
const keyNamespace = "github.com/altessa-s/go-atlas/transport/grpc/interceptors/idempotency"

// DeriveKey returns a deterministic idempotency key for a single outbound mutating call.
// It is derived from seed — a stable identifier of the logical operation that stays
// constant across its retries — and call, a name identifying the specific downstream call.
//
// The same (seed, call) pair always yields the same key, so retries of one logical call
// are deduplicated by the server. Different call names yield different keys, so several
// downstream calls made while handling one operation do not collide — the server scopes
// idempotency per service, not per method.
//
// The result is a lowercase UUID v4 accepted by [DefaultKeyValidator].
//
// Example:
//
//	key := idempotency.DeriveKey(operationID, "users.UserService/Update")
func DeriveKey(seed, call string) string {
	sum := sha256.Sum256([]byte(keyNamespace + "\x00" + seed + "\x00" + call))
	return uuid.Must(uuid.NewRandomFromReader(bytes.NewReader(sum[:]))).String()
}

// WithKey returns a copy of ctx with key attached as outgoing gRPC metadata under
// [DefaultIdempotencyKeyHeader], preserving any existing outgoing metadata. Use it when
// the caller already holds an idempotency key; otherwise use [WithDerivedKey].
func WithKey(ctx context.Context, key string) context.Context {
	md, ok := metadata.FromOutgoingContext(ctx)
	if ok {
		md = md.Copy()
	} else {
		md = metadata.New(nil)
	}

	md.Set(DefaultIdempotencyKeyHeader, key)
	return metadata.NewOutgoingContext(ctx, md)
}

// WithDerivedKey attaches a key produced by [DeriveKey] to ctx as outgoing gRPC metadata.
// It is shorthand for WithKey(ctx, DeriveKey(seed, call)).
//
// Example:
//
//	ctx = idempotency.WithDerivedKey(ctx, operationID, "users.UserService/Update")
//	resp, err := client.Update(ctx, req)
func WithDerivedKey(ctx context.Context, seed, call string) context.Context {
	return WithKey(ctx, DeriveKey(seed, call))
}
