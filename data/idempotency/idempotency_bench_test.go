// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"fmt"
	"testing"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func BenchmarkAttemptLock(b *testing.B) {
	s := testhelpers.NewMockIdempotencyStorage()
	k := New(s)
	ctx := b.Context()

	b.ResetTimer()
	i := 0
	for b.Loop() {
		_, _, _ = k.AttemptLock(ctx, fmt.Sprintf("key-%d", i))
		i++
	}
}
