// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cuckoo_test

import (
	"log/slog"
	"testing"

	"github.com/altessa-s/go-atlas/data/probfilter/cuckoo"
	"github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages/memory"
)

func TestNew_WithLogger(t *testing.T) {
	storage := memory.New(memory.WithCapacity(100))
	f := cuckoo.New(storage, cuckoo.WithLogger(slog.Default()))
	if f == nil {
		t.Fatal("New(WithLogger) returned nil")
	}
}

func TestFilter_Stats_Complete(t *testing.T) {
	storage := memory.New(memory.WithCapacity(100))
	f := cuckoo.New(storage)
	ctx := t.Context()

	// Add some items
	_ = f.Add(ctx, "a")
	_ = f.Add(ctx, "b")

	stats, err := f.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats() error = %v", err)
	}
	if stats == nil {
		t.Fatal("Stats() returned nil")
	}
}

func TestFilter_DeleteAndRecheck(t *testing.T) {
	storage := memory.New(memory.WithCapacity(100))
	f := cuckoo.New(storage)
	ctx := t.Context()

	_ = f.Add(ctx, "item1")

	ok, err := f.Delete(ctx, "item1")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if !ok {
		t.Error("Delete() should return true for existing item")
	}

	// After delete, MightExist should return false
	exists, err := f.MightExist(ctx, "item1")
	if err != nil {
		t.Fatalf("MightExist() error = %v", err)
	}
	if exists {
		t.Error("MightExist() should return false after Delete")
	}
}

func TestFilter_AddBatch_Multiple(t *testing.T) {
	storage := memory.New(memory.WithCapacity(1000))
	f := cuckoo.New(storage)
	ctx := t.Context()

	items := func(yield func(string) bool) {
		for i := range 100 {
			if !yield("item-" + string(rune('a'+i%26))) {
				return
			}
		}
	}

	if err := f.AddBatch(ctx, items); err != nil {
		t.Fatalf("AddBatch() error = %v", err)
	}
}
