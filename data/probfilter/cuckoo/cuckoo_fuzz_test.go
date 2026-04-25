// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cuckoo_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/probfilter/cuckoo"
	"github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages/memory"
)

func FuzzFilter_AddMightExist(f *testing.F) {
	f.Add("hello")
	f.Add("")
	f.Add("special-chars!@#$%")

	s := memory.New(memory.WithCapacity(100000))
	filter := cuckoo.New(s)
	ctx := f.Context()

	f.Fuzz(func(t *testing.T, value string) {
		// Should not panic
		_ = filter.Add(ctx, value)
		_, _ = filter.MightExist(ctx, value)
		_, _ = filter.Delete(ctx, value)
	})
}
