// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package probfilterit_test

import (
	"strconv"
	"testing"

	cuckooredis "github.com/altessa-s/go-atlas/cache/probfilter/cuckoo/storages/redis"
)

// BenchmarkCuckoo_Add compares the journaling add script with a plain CF.ADD
// against the same filter.
func BenchmarkCuckoo_Add(b *testing.B) {
	fx := newFixture(b)
	f := fx.cuckoo(b, cuckooredis.WithCapacity(1<<22))
	ctx := b.Context()

	b.Run("script", func(b *testing.B) {
		i := 0
		for b.Loop() {
			if err := f.Add(ctx, "script-"+strconv.Itoa(i)); err != nil {
				b.Fatal(err)
			}
			i++
		}
	})

	b.Run("plain CF.ADD", func(b *testing.B) {
		i := 0
		for b.Loop() {
			if err := fx.admin.Do(ctx, "CF.ADD", fx.liveKey(), "plain-"+strconv.Itoa(i)).Err(); err != nil {
				b.Fatal(err)
			}
			i++
		}
	})
}
