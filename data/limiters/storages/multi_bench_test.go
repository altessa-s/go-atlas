// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package storages_test

import (
	"strconv"
	"testing"

	"github.com/altessa-s/go-atlas/data/limiters/storages"
)

// BenchmarkValidateLimits measures the per-call argument check of a
// multi-key admission, on the pairwise and the set path.
func BenchmarkValidateLimits(b *testing.B) {
	for _, n := range []int{2, 8, 16} {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			keys := make([]string, n)
			for i := range keys {
				keys[i] = "key-" + strconv.Itoa(i)
			}
			limits := limitsOf(keys...)
			b.ReportAllocs()
			for b.Loop() {
				_ = storages.ValidateLimits(limits)
			}
		})
	}
}
