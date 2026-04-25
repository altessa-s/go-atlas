// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package probfilter_test

import (
	"fmt"
	"testing"

	"github.com/altessa-s/go-atlas/data/probfilter"
	"github.com/altessa-s/go-atlas/data/probfilter/bloom"

	bloommemory "github.com/altessa-s/go-atlas/data/probfilter/bloom/storages/memory"
)

func BenchmarkManager_Get(b *testing.B) {
	mgr := probfilter.NewManager()
	storage := bloommemory.New(bloommemory.WithExpectedItems(1000))
	filter := bloom.New(storage)
	_ = mgr.Register("test", filter)

	b.ResetTimer()
	for b.Loop() {
		_, _ = mgr.Get("test")
	}
}

func BenchmarkFilter_MightExist(b *testing.B) {
	ctx := b.Context()
	storage := bloommemory.New(bloommemory.WithExpectedItems(10000))
	f := bloom.New(storage)

	for i := range 1000 {
		_ = f.Add(ctx, fmt.Sprintf("item-%d", i))
	}

	b.ResetTimer()
	i := 0
	for b.Loop() {
		_, _ = f.MightExist(ctx, fmt.Sprintf("item-%d", i%1000))
		i++
	}
}
