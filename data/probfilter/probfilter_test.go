// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package probfilter_test

import (
	"context"
	"iter"
	"slices"
	"testing"

	"github.com/altessa-s/go-atlas/data/probfilter"
	"github.com/altessa-s/go-atlas/data/probfilter/bloom"
	"github.com/altessa-s/go-atlas/data/probfilter/cuckoo"

	bloommemory "github.com/altessa-s/go-atlas/data/probfilter/bloom/storages/memory"
	cuckoomemory "github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages/memory"
)

func TestManager(t *testing.T) {
	mgr := probfilter.NewManager()

	bStorage := bloommemory.New(bloommemory.WithExpectedItems(100))
	bFilter := bloom.New(bStorage)

	cStorage := cuckoomemory.New(cuckoomemory.WithCapacity(100))
	cFilter := cuckoo.New(cStorage)

	if err := mgr.Register("bloom", bFilter); err != nil {
		t.Fatalf("failed to register bloom: %v", err)
	}

	if err := mgr.Register("cuckoo", cFilter); err != nil {
		t.Fatalf("failed to register cuckoo: %v", err)
	}

	// Test Names iterator
	var names []string
	for name := range mgr.Names() {
		names = append(names, name)
	}
	slices.Sort(names)
	expectedNames := []string{"bloom", "cuckoo"}
	if !slices.Equal(names, expectedNames) {
		t.Errorf("expected names %v, got %v", expectedNames, names)
	}

	// Test Filters iterator
	count := 0
	for name, f := range mgr.Filters() {
		if name == "" || f == nil {
			t.Error("empty name or nil filter in iterator")
		}
		count++
	}
	if count != 2 {
		t.Errorf("expected 2 filters, got %d", count)
	}

	if err := mgr.Close(); err != nil {
		t.Errorf("failed to close manager: %v", err)
	}
}

func TestBloomFilter_AddBatch(t *testing.T) {
	ctx := t.Context()
	storage := bloommemory.New(bloommemory.WithExpectedItems(1000))
	filter := bloom.New(storage)

	values := []string{"a", "b", "c"}
	iter := func(yield func(string) bool) {
		for _, v := range values {
			if !yield(v) {
				return
			}
		}
	}

	if err := filter.AddBatch(ctx, iter); err != nil {
		t.Fatalf("failed to add batch: %v", err)
	}

	for _, v := range values {
		exists, err := filter.MightExist(ctx, v)
		if err != nil {
			t.Errorf("failed to check existence for %s: %v", v, err)
		}
		if !exists {
			t.Errorf("expected value %s to exist", v)
		}
	}
}

func TestCuckooFilter_AddBatch(t *testing.T) {
	ctx := t.Context()
	storage := cuckoomemory.New(cuckoomemory.WithCapacity(1000))
	filter := cuckoo.New(storage)

	values := []string{"x", "y", "z"}
	iter := func(yield func(string) bool) {
		for _, v := range values {
			if !yield(v) {
				return
			}
		}
	}

	if err := filter.AddBatch(ctx, iter); err != nil {
		t.Fatalf("failed to add batch: %v", err)
	}

	for _, v := range values {
		exists, err := filter.MightExist(ctx, v)
		if err != nil {
			t.Errorf("failed to check existence for %s: %v", v, err)
		}
		if !exists {
			t.Errorf("expected value %s to exist", v)
		}
	}

	// Test Delete
	ok, err := filter.Delete(ctx, "x")
	if err != nil {
		t.Errorf("failed to delete: %v", err)
	}
	if !ok {
		t.Error("expected delete to return true")
	}

	exists, _ := filter.MightExist(ctx, "x")
	if exists {
		t.Error("expected value x to not exist after delete")
	}
}

type mockLoader struct {
	countCalled  int
	streamCalled int
	items        []string
}

func (m *mockLoader) Count(ctx context.Context) (int64, error) {
	m.countCalled++
	return int64(len(m.items)), nil
}

func (m *mockLoader) StreamValues(ctx context.Context) iter.Seq2[string, error] {
	m.streamCalled++
	return func(yield func(string, error) bool) {
		for _, v := range m.items {
			if !yield(v, nil) {
				return
			}
		}
	}
}

func TestBloomFilter_Rebuild_Optimization(t *testing.T) {
	ctx := t.Context()
	storage := bloommemory.New(bloommemory.WithExpectedItems(100))
	filter := bloom.New(storage)

	loader := &mockLoader{
		items: []string{"a", "b", "c"},
	}

	if err := filter.Rebuild(ctx, loader); err != nil {
		t.Fatalf("failed to rebuild: %v", err)
	}

	// Verify optimization: Count should be called once, StreamValues once (for loading)
	// If optimization wasn't working, StreamValues might be called twice (once for count, once for load)
	if loader.countCalled != 1 {
		t.Errorf("expected Count to be called 1 time, got %d", loader.countCalled)
	}
	if loader.streamCalled != 1 {
		t.Errorf("expected StreamValues to be called 1 time, got %d", loader.streamCalled)
	}

	// Verify items were loaded
	for _, v := range loader.items {
		exists, _ := filter.MightExist(ctx, v)
		if !exists {
			t.Errorf("expected value %s to exist after rebuild", v)
		}
	}
}
