// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency_test

import (
	"context"
	"fmt"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/idempotency"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// ExampleDeriveKey demonstrates the two load-bearing invariants of [DeriveKey]:
// the same (seed, call) pair always yields the same key, and a change in
// either input yields a different key. The actual UUID value is intentionally
// not printed — the property matters, the byte sequence does not.
func ExampleDeriveKey() {
	const call = "/users.v1.UserService/Update"

	a := idempotency.DeriveKey("operation-1", call)
	b := idempotency.DeriveKey("operation-1", call)
	c := idempotency.DeriveKey("operation-2", call)
	d := idempotency.DeriveKey("operation-1", "/users.v1.UserService/Delete")

	fmt.Println("retry of same op:", a == b)
	fmt.Println("different seed:  ", a == c)
	fmt.Println("different call:  ", a == d)
	// Output:
	// retry of same op: true
	// different seed:   false
	// different call:   false
}

// ExampleWithOperation shows how to tag a context with an operation seed so
// downstream gRPC calls handled by [UnaryClientInterceptor] inherit a
// deterministic Idempotency-Key.
func ExampleWithOperation() {
	ctx := context.Background()

	// External boundary attaches the operation seed once.
	ctx = idempotency.WithOperation(ctx, "operation-3b24398f-257d-4879-8ba1-89cb176abe72")

	// Downstream code can re-read the seed without knowing the context key.
	if op, ok := idempotency.OperationFromContext(ctx); ok {
		fmt.Println("operation:", op)
	}
	// Output: operation: operation-3b24398f-257d-4879-8ba1-89cb176abe72
}

// ExampleWithDerivedKey shows the manual path for stamping a single outbound
// call without installing [UnaryClientInterceptor]. The derived key is placed
// under [DefaultIdempotencyKeyHeader] on the outgoing context.
func ExampleWithDerivedKey() {
	ctx := idempotency.WithDerivedKey(context.Background(),
		"operation-1", "/users.v1.UserService/Update")

	md, _ := metadata.FromOutgoingContext(ctx)
	fmt.Println("header attached:", len(md.Get(idempotency.DefaultIdempotencyKeyHeader)) == 1)
	// Output: header attached: true
}

// ExampleUnaryClientInterceptor shows the canonical wiring. After the call
// context is tagged with [WithOperation], the interceptor stamps an
// Idempotency-Key derived from the operation ID and the full method name.
// A fake invoker stands in for the real gRPC call here so the example is
// self-contained.
func ExampleUnaryClientInterceptor() {
	interceptor := idempotency.UnaryClientInterceptor()

	ctx := idempotency.WithOperation(context.Background(),
		"operation-3b24398f-257d-4879-8ba1-89cb176abe72")

	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		md, _ := metadata.FromOutgoingContext(ctx)
		fmt.Println("Idempotency-Key stamped:",
			len(md.Get(idempotency.DefaultIdempotencyKeyHeader)) == 1)
		return nil
	}

	_ = interceptor(ctx, "/users.v1.UserService/Update", nil, nil, nil, invoker)
	// Output: Idempotency-Key stamped: true
}
