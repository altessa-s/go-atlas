// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/transport/internal/idempotency"
)

func BenchmarkDefaultKeyValidator(b *testing.B) {
	for b.Loop() {
		_ = idempotency.DefaultKeyValidator("550e8400-e29b-41d4-a716-446655440000")
	}
}

func BenchmarkBuildStorageKey(b *testing.B) {
	for b.Loop() {
		_ = idempotency.BuildStorageKey("users.UserService", "550e8400-e29b-41d4-a716-446655440000")
	}
}
