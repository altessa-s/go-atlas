// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisbase_test

import (
	"testing"
)

func BenchmarkBase_Key(b *testing.B) {
	base, _ := setupBase(b, "myapp:cache")
	for b.Loop() {
		base.Key("user:123")
	}
}

func BenchmarkBase_BuildKeys(b *testing.B) {
	base, _ := setupBase(b, "myapp:cache")
	keys := []string{"k1", "k2", "k3", "k4", "k5"}
	for b.Loop() {
		base.BuildKeys(keys...)
	}
}

func BenchmarkBase_Exists(b *testing.B) {
	base, mr := setupBase(b, "")
	mr.Set("bench-key", "val")
	ctx := b.Context()
	for b.Loop() {
		_, _ = base.Exists(ctx, "bench-key")
	}
}

func BenchmarkBase_Delete(b *testing.B) {
	base, _ := setupBase(b, "")
	ctx := b.Context()
	for b.Loop() {
		_ = base.Delete(ctx, "bench-key")
	}
}
