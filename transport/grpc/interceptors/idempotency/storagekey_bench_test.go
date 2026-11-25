// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import "testing"

// storageKeySink keeps the built keys escaping, so the benchmark counts the
// allocation a request pays.
var storageKeySink string

func BenchmarkBuildStorageKey(b *testing.B) {
	ri := &requestInterceptor{}
	b.ReportAllocs()
	for b.Loop() {
		storageKeySink = ri.buildKey("users.UserService", "550e8400-e29b-41d4-a716-446655440000")
	}
}
