// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package shared

import (
	"sync"
	"testing"
)

func TestGetOrCreate(t *testing.T) {
	var m sync.Map
	calls := 0
	create := func() string {
		calls++
		return "value"
	}

	v1 := GetOrCreate(&m, "key", create)
	v2 := GetOrCreate(&m, "key", create)

	if v1 != "value" || v2 != "value" {
		t.Errorf("v1=%q, v2=%q", v1, v2)
	}
	if calls != 1 {
		t.Errorf("create called %d times, want 1", calls)
	}
}

func TestGetOrCreateWithCallback(t *testing.T) {
	var m sync.Map
	callbacks := 0

	v := GetOrCreateWithCallback(&m, "key",
		func() int { return 42 },
		func() { callbacks++ },
	)
	if v != 42 {
		t.Errorf("v = %d", v)
	}
	if callbacks != 1 {
		t.Errorf("callbacks = %d", callbacks)
	}

	// Second call should not trigger callback
	v2 := GetOrCreateWithCallback(&m, "key",
		func() int { return 99 },
		func() { callbacks++ },
	)
	if v2 != 42 {
		t.Errorf("v2 = %d, want 42 (original)", v2)
	}
	if callbacks != 1 {
		t.Errorf("callbacks = %d, want 1", callbacks)
	}
}

func TestGetOrCreateWithCallback_NilCallback(t *testing.T) {
	var m sync.Map
	v := GetOrCreateWithCallback(&m, "key",
		func() string { return "val" },
		nil,
	)
	if v != "val" {
		t.Errorf("v = %q", v)
	}
}

func TestRange(t *testing.T) {
	var m sync.Map
	m.Store("a", "1")
	m.Store("b", "2")
	m.Store(42, "skip") // non-string key

	collected := make(map[string]string)
	Range(&m, func(key string, value string) bool {
		collected[key] = value
		return true
	})

	if len(collected) != 2 {
		t.Errorf("collected %d items, want 2", len(collected))
	}
	if collected["a"] != "1" || collected["b"] != "2" {
		t.Errorf("collected = %v", collected)
	}
}

func TestRange_EarlyExit(t *testing.T) {
	var m sync.Map
	m.Store("a", "1")
	m.Store("b", "2")
	m.Store("c", "3")

	count := 0
	Range(&m, func(_ string, _ string) bool {
		count++
		return false // stop after first
	})
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestGetOrCreate_Concurrent(t *testing.T) {
	var m sync.Map
	var wg sync.WaitGroup
	createCalls := int32(0)

	for range 100 {
		wg.Go(func() {
			GetOrCreate(&m, "key", func() string {
				sync.OnceFunc(func() {})() // just to add some contention
				return "value"
			})
			_ = createCalls // just to avoid lint
		})
	}
	wg.Wait()

	v, ok := m.Load("key")
	if !ok || v.(string) != "value" {
		t.Errorf("unexpected value: %v", v)
	}
}
