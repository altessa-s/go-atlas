// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package shared

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
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

	require.Equal(t, "value", v1)
	require.Equal(t, "value", v2)
	require.Equal(t, 1, calls, "create should be called once")
}

func TestGetOrCreateWithCallback(t *testing.T) {
	var m sync.Map
	callbacks := 0

	v := GetOrCreateWithCallback(&m, "key",
		func() int { return 42 },
		func() { callbacks++ },
	)
	require.Equal(t, 42, v)
	require.Equal(t, 1, callbacks)

	// Second call should not trigger callback
	v2 := GetOrCreateWithCallback(&m, "key",
		func() int { return 99 },
		func() { callbacks++ },
	)
	require.Equal(t, 42, v2, "should return original value")
	require.Equal(t, 1, callbacks, "callback should not be called again")
}

func TestGetOrCreateWithCallback_NilCallback(t *testing.T) {
	var m sync.Map
	v := GetOrCreateWithCallback(&m, "key",
		func() string { return "val" },
		nil,
	)
	require.Equal(t, "val", v)
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

	require.Len(t, collected, 2)
	require.Equal(t, "1", collected["a"])
	require.Equal(t, "2", collected["b"])
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
	require.Equal(t, 1, count)
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
	require.True(t, ok)
	require.Equal(t, "value", v.(string))
}
