// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/idempotency"
)

// composedKeySink keeps the composed keys escaping, so the benchmarks count
// the allocation a storage call pays.
var composedKeySink string

func BenchmarkComposeKey(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		composedKeySink = idempotency.ComposeKey("transport", "POST:/api/v1/users", "550e8400-e29b-41d4-a716-446655440000")
	}
}

// BenchmarkComposeKeyNested composes a transport key and wraps it in a Keeper
// namespace, the two compositions an HTTP or gRPC request pays.
func BenchmarkComposeKeyNested(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		inner := idempotency.ComposeKey("transport", "users.UserService", "550e8400-e29b-41d4-a716-446655440000")
		composedKeySink = idempotency.ComposeKey("namespace", "acme", inner)
	}
}
