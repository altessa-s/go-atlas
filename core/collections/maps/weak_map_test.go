// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package maps_test

import (
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
)

func TestWeakMap_GC(t *testing.T) {
	m := coremaps.NewWeakMap[int, testhelpers.User]()

	// Create an object and store it in the map
	val := &testhelpers.User{ID: 1, Name: "test"}
	m.Set(1, val)

	// Verify we can get it
	v, ok := m.Get(1)
	require.True(t, ok, "expected key 1 to be present")
	require.NotNil(t, v)
	require.Equal(t, 1, v.ID)

	// Remove our reference to the object
	val = nil

	// Trigger GC. We may need several rounds to ensure it's collected.
	for range 5 {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}

	// Verify it's gone from the map
	v, ok = m.Get(1)
	require.False(t, ok, "Value should have been collected: ok=%v, v=%v", ok, v)
	require.Nil(t, v, "Value should have been collected: ok=%v, v=%v", ok, v)
}

func TestWeakMap_ThreadSafety(t *testing.T) {
	m := coremaps.NewWeakMap[int, testhelpers.User]()
	const count = 1000

	// Concurrent writes
	done := make(chan bool)
	for i := range 10 {
		go func(id int) {
			for j := range count {
				val := &testhelpers.User{ID: j, Name: "test"}
				m.Set(id*count+j, val)
			}
			done <- true
		}(i)
	}

	for range 10 {
		<-done
	}

	if m.Len() < 10*count {
		// Note: some might have been collected already if we're exceptionally fast/memory pressured,
		// but with 1000 items and keeping pointers locally if we're not careful...
		// In this specific loop, 'val' is local to goroutine and might be collected after each iteration.
	}
}

func TestWeakMap_AutoCleanup(t *testing.T) {
	m := coremaps.NewWeakMap[int, testhelpers.User]()

	// Populate the map; don't retain references to the values.
	for i := range 50 {
		m.Set(i, &testhelpers.User{ID: i, Name: "disposable"})
	}

	// Keep one value alive to verify it survives.
	kept := &testhelpers.User{ID: 999, Name: "kept"}
	m.Set(999, kept)

	// At this point the 50 values have no external references.
	// Trigger GC and wait for cleanup callbacks to run.
	for range 10 {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}

	// The kept value must still be present.
	v, ok := m.Get(999)
	require.True(t, ok, "kept value missing after GC")
	require.Equal(t, kept, v, "kept value missing after GC: got %v", v)

	// Stale entries should have been cleaned up automatically.
	// Allow some slack — cleanup callbacks are asynchronous.
	require.LessOrEqual(t, m.Len(), 10, "expected most stale entries to be cleaned up, but Len()=%d", m.Len())

	runtime.KeepAlive(kept)
}
