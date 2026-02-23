// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cuckoo_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/probfilter/cuckoo"
	"github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages/memory"
)

func TestNew(t *testing.T) {
	s := memory.New(memory.WithCapacity(1000))
	f := cuckoo.New(s)
	if f == nil {
		t.Fatal("New() returned nil")
	}
}

func TestFilter_AddAndMightExist(t *testing.T) {
	s := memory.New(memory.WithCapacity(1000))
	f := cuckoo.New(s)
	ctx := t.Context()

	if err := f.Add(ctx, "hello"); err != nil {
		t.Fatalf("Add() error: %v", err)
	}

	exists, err := f.MightExist(ctx, "hello")
	if err != nil {
		t.Fatalf("MightExist() error: %v", err)
	}
	if !exists {
		t.Error("MightExist() should return true for added item")
	}
}

func TestFilter_MightExist_NotAdded(t *testing.T) {
	s := memory.New(memory.WithCapacity(1000))
	f := cuckoo.New(s)
	ctx := t.Context()

	exists, err := f.MightExist(ctx, "notadded")
	if err != nil {
		t.Fatalf("MightExist() error: %v", err)
	}
	if exists {
		t.Error("MightExist() should return false for non-existent item")
	}
}

func TestFilter_AddBatch(t *testing.T) {
	s := memory.New(memory.WithCapacity(1000))
	f := cuckoo.New(s)
	ctx := t.Context()

	values := func(yield func(string) bool) {
		for _, v := range []string{"a", "b", "c"} {
			if !yield(v) {
				return
			}
		}
	}

	if err := f.AddBatch(ctx, values); err != nil {
		t.Fatalf("AddBatch() error: %v", err)
	}

	for _, v := range []string{"a", "b", "c"} {
		exists, _ := f.MightExist(ctx, v)
		if !exists {
			t.Errorf("MightExist(%q) = false, want true", v)
		}
	}
}

func TestFilter_Delete(t *testing.T) {
	s := memory.New(memory.WithCapacity(1000))
	f := cuckoo.New(s)
	ctx := t.Context()

	_ = f.Add(ctx, "item")
	deleted, err := f.Delete(ctx, "item")
	if err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
	if !deleted {
		t.Error("Delete() should return true for existing item")
	}

	exists, _ := f.MightExist(ctx, "item")
	if exists {
		t.Error("MightExist() should return false after Delete()")
	}
}

func TestFilter_Delete_NotExist(t *testing.T) {
	s := memory.New(memory.WithCapacity(1000))
	f := cuckoo.New(s)
	ctx := t.Context()

	deleted, err := f.Delete(ctx, "nope")
	if err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
	if deleted {
		t.Error("Delete() should return false for non-existent item")
	}
}

func TestFilter_Stats(t *testing.T) {
	s := memory.New(memory.WithCapacity(1000))
	f := cuckoo.New(s)
	ctx := t.Context()

	_ = f.Add(ctx, "x")
	stats, err := f.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats() error: %v", err)
	}
	if stats.StorageType != "memory" {
		t.Errorf("StorageType = %q, want %q", stats.StorageType, "memory")
	}
}

func TestFilter_Close(t *testing.T) {
	s := memory.New(memory.WithCapacity(1000))
	f := cuckoo.New(s)
	if err := f.Close(t.Context()); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
}
