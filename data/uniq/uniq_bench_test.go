// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package uniq

import (
	"fmt"
	"testing"
)

func BenchmarkAdd(b *testing.B) {
	u := NewWithNoop()
	ctx := b.Context()

	b.ResetTimer()
	i := 0
	for b.Loop() {
		_ = u.Add(ctx, fmt.Sprintf("key-%d", i))
		i++
	}
}

func BenchmarkExist(b *testing.B) {
	u := NewWithNoop()
	ctx := b.Context()

	b.ResetTimer()
	i := 0
	for b.Loop() {
		_, _ = u.Exist(ctx, fmt.Sprintf("key-%d", i))
		i++
	}
}
