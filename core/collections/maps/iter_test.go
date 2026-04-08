// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package maps_test

import (
	"testing"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
)

func TestKeys(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}
	got := make(map[string]bool)
	for k := range coremaps.Keys(m) {
		got[k] = true
	}
	if !got["a"] || !got["b"] {
		t.Errorf("Keys() missing expected keys, got %v", got)
	}
}

func TestValues(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}
	sum := 0
	for v := range coremaps.Values(m) {
		sum += v
	}
	if sum != 3 {
		t.Errorf("Values() sum = %d, want 3", sum)
	}
}

func TestFilter(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2, "c": 3}
	got := make(map[string]int)
	for k, v := range coremaps.Filter(m, func(k string, v int) bool { return v > 1 }) {
		got[k] = v
	}
	if len(got) != 2 {
		t.Errorf("Filter() len = %d, want 2", len(got))
	}
	if _, ok := got["a"]; ok {
		t.Error("Filter() should not include a=1")
	}
}

func TestMapIter(t *testing.T) {
	m := map[string]int{"a": 1}
	got := make(map[string]string)
	for k, v := range coremaps.Map(m, func(k string, v int) (string, string) {
		return k + "!", "val"
	}) {
		got[k] = v
	}
	if got["a!"] != "val" {
		t.Errorf("Map() = %v", got)
	}
}

func TestPool(t *testing.T) {
	pool := coremaps.NewPool[string, int](10)
	m := pool.Get()
	if m == nil {
		t.Fatal("Get() returned nil")
	}
	(*m)["key"] = 42
	pool.Put(m)

	m2 := pool.GetWithCapacity(50)
	if m2 == nil {
		t.Fatal("GetWithCapacity() returned nil")
	}
	pool.Put(m2)
}

// TestPool_GetWithCapacity_ReusesWithinDefault is a regression test for a
// bug where GetWithCapacity always reallocated: it compared expectedCapacity
// against len(*m), which is zero after Get's clear(), so every call threw
// away the pooled map and returned a fresh allocation. The fix fast-paths
// requests within defaultCap by returning the pooled pointer untouched.
func TestPool_GetWithCapacity_ReusesWithinDefault(t *testing.T) {
	pool := coremaps.NewPool[string, int](64)

	// Seed the pool with a known map pointer.
	seeded := pool.Get()
	(*seeded)["seed"] = 1
	pool.Put(seeded)

	// A request within defaultCap must reuse the pooled allocation.
	reused := pool.GetWithCapacity(32)
	if reused != seeded {
		t.Errorf("GetWithCapacity(<=defaultCap) allocated a new map; "+
			"want reuse of pooled pointer %p, got %p", seeded, reused)
	}
	if len(*reused) != 0 {
		t.Errorf("reused map not cleared: len=%d", len(*reused))
	}
	pool.Put(reused)

	// A request exceeding defaultCap legitimately discards the pooled
	// map and allocates a new one — Go cannot expose allocated capacity
	// at runtime, so the pooled map might not satisfy the request.
	oversize := pool.GetWithCapacity(256)
	if oversize == nil {
		t.Fatal("GetWithCapacity(oversize) returned nil")
	}
	pool.Put(oversize)
}

func TestWeakRef(t *testing.T) {
	val := 42
	ref := coremaps.MakeWeakRef(&val)
	if !ref.IsAlive() {
		t.Error("IsAlive() should be true for live reference")
	}
	if got := ref.Value(); got == nil || *got != 42 {
		t.Errorf("Value() = %v, want *42", got)
	}
}

func TestWeakMap_Basic(t *testing.T) {
	wm := coremaps.NewWeakMap[string, int]()

	val := 42
	wm.Set("key", &val)

	got, ok := wm.Get("key")
	if !ok || got == nil || *got != 42 {
		t.Errorf("Get(key) = %v, %v, want *42, true", got, ok)
	}

	if wm.Len() != 1 {
		t.Errorf("Len() = %d, want 1", wm.Len())
	}

	wm.Delete("key")
	_, ok = wm.Get("key")
	if ok {
		t.Error("Get(key) should return false after Delete")
	}
}

func TestWeakMap_Range(t *testing.T) {
	wm := coremaps.NewWeakMap[string, int]()
	v1 := 1
	v2 := 2
	wm.Set("a", &v1)
	wm.Set("b", &v2)

	count := 0
	wm.Range(func(key string, value *int) bool {
		count++
		return true
	})
	if count != 2 {
		t.Errorf("Range() visited %d entries, want 2", count)
	}
}

func TestWeakMap_Cleanup(t *testing.T) {
	wm := coremaps.NewWeakMap[string, int]()
	v := 42
	wm.Set("key", &v)
	wm.Cleanup() // should not panic
}
