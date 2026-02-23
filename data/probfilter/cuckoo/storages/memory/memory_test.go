// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"testing"

	memory "github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages/memory"
)

func TestStorage_AddAndMightExist(t *testing.T) {
	storage := memory.New(memory.WithCapacity(1000))
	ctx := t.Context()

	err := storage.Add(ctx, "hello")
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	exists, err := storage.MightExist(ctx, "hello")
	if err != nil {
		t.Fatalf("MightExist failed: %v", err)
	}
	if !exists {
		t.Error("Expected 'hello' to exist in filter")
	}
}

func TestStorage_MightExist_NotAdded(t *testing.T) {
	storage := memory.New(memory.WithCapacity(1000))
	ctx := t.Context()

	exists, err := storage.MightExist(ctx, "notadded")
	if err != nil {
		t.Fatalf("MightExist failed: %v", err)
	}
	if exists {
		t.Error("Expected 'notadded' to not exist in filter")
	}
}

func TestStorage_AddBatch(t *testing.T) {
	storage := memory.New(memory.WithCapacity(1000))
	ctx := t.Context()

	values := func(yield func(string) bool) {
		items := []string{"item1", "item2", "item3"}
		for _, item := range items {
			if !yield(item) {
				return
			}
		}
	}

	err := storage.AddBatch(ctx, values)
	if err != nil {
		t.Fatalf("AddBatch failed: %v", err)
	}

	for _, item := range []string{"item1", "item2", "item3"} {
		exists, err := storage.MightExist(ctx, item)
		if err != nil {
			t.Fatalf("MightExist failed for %s: %v", item, err)
		}
		if !exists {
			t.Errorf("Expected '%s' to exist in filter", item)
		}
	}
}

func TestStorage_Delete(t *testing.T) {
	storage := memory.New(memory.WithCapacity(1000))
	ctx := t.Context()

	err := storage.Add(ctx, "item")
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	deleted, err := storage.Delete(ctx, "item")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if !deleted {
		t.Error("Expected Delete to return true for existing item")
	}

	exists, err := storage.MightExist(ctx, "item")
	if err != nil {
		t.Fatalf("MightExist failed: %v", err)
	}
	if exists {
		t.Error("Expected 'item' to not exist after deletion")
	}
}

func TestStorage_Delete_NotExist(t *testing.T) {
	storage := memory.New(memory.WithCapacity(1000))
	ctx := t.Context()

	deleted, err := storage.Delete(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if deleted {
		t.Error("Expected Delete to return false for non-existent item")
	}
}

func TestStorage_Stats(t *testing.T) {
	storage := memory.New(memory.WithCapacity(1000))
	ctx := t.Context()

	for i := range 10 {
		err := storage.Add(ctx, string(rune('a'+i)))
		if err != nil {
			t.Fatalf("Add failed: %v", err)
		}
	}

	stats, err := storage.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}

	if stats.Capacity <= 0 {
		t.Errorf("Expected capacity > 0, got %d", stats.Capacity)
	}

	if stats.StorageType != "memory" {
		t.Errorf("Expected storageType 'memory', got '%s'", stats.StorageType)
	}
}

func TestStorage_Close(t *testing.T) {
	storage := memory.New(memory.WithCapacity(1000))
	ctx := t.Context()

	err := storage.Close(ctx)
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}
