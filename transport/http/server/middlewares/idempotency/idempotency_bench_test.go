// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import "testing"

func BenchmarkDefaultKeyValidator(b *testing.B) {
	key := "550e8400-e29b-41d4-a716-446655440000"
	for b.Loop() {
		DefaultKeyValidator(key) //nolint:errcheck
	}
}

func BenchmarkBuildKey(b *testing.B) {
	m := &middleware{}
	for b.Loop() {
		m.buildKey("POST", "/api/v1/users", "550e8400-e29b-41d4-a716-446655440000")
	}
}
