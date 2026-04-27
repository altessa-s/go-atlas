// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"fmt"
	"testing"
)

func BenchmarkStorage_AttemptLock(b *testing.B) {
	s := New()
	ctx := b.Context()
	b.ResetTimer()
	for b.Loop() {
		s.AttemptLock(ctx, "bench-key", []byte("val"))
		s.Delete(ctx, "bench-key")
	}
}

func BenchmarkStorage_Complete(b *testing.B) {
	s := New()
	ctx := b.Context()
	_, _, lockToken, _ := s.AttemptLock(ctx, "bench-key", []byte("in-progress"))
	b.ResetTimer()
	for b.Loop() {
		s.Complete(ctx, "bench-key", []byte("done"), lockToken)
	}
}

func BenchmarkStorage_AttemptLock_Contention(b *testing.B) {
	s := New()
	ctx := b.Context()
	s.AttemptLock(ctx, "existing", []byte("val"))
	b.ResetTimer()
	for b.Loop() {
		s.AttemptLock(ctx, "existing", []byte("val2"))
	}
}

func BenchmarkStorage_RunCleanup(b *testing.B) {
	s := New(WithTtl(1))
	ctx := b.Context()
	for i := range 1000 {
		s.AttemptLock(ctx, fmt.Sprintf("key-%d", i), []byte("val"))
	}
	b.ResetTimer()
	for b.Loop() {
		s.RunCleanup()
	}
}
