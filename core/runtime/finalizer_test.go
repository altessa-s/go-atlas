// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package runtime

import (
	"runtime"
	"testing"
	"time"
)

// cleanupWait bounds how long a test waits for a cleanup to fire after the GC
// has observed the object as unreachable. Cleanups run on a dedicated
// goroutine, so the wait is for scheduling rather than for collection.
const cleanupWait = 5 * time.Second

// tracked is deliberately larger than the 16-byte tiny-allocation threshold.
//
// Go 1.26 panics when a cleanup function closes over the object it is attached
// to, because such a closure keeps the object alive and the cleanup could never
// run. The check compares allocations, and the tiny allocator packs several
// small pointer-free objects into one block — so a `new(int)` object and a
// captured `bool` can land in the same allocation and trip the check even
// though the closure never mentions the object. Allocating out of the tiny
// range keeps the test measuring the wrapper instead of the allocator.
type tracked struct{ _ [4]int64 }

func TestAddCleanup(t *testing.T) {
	t.Run("runs once the object is unreachable", func(t *testing.T) {
		// Buffered so the cleanup goroutine never blocks on a receiver that
		// timed out, which would leak it for the rest of the run.
		fired := make(chan bool, 1)

		obj := new(tracked)
		cleanup := AddCleanup(obj, func(v bool) { fired <- v }, true)
		if cleanup == nil {
			t.Fatal("expected non-nil Cleanup")
		}

		// KeepAlive marks the last point obj is required, so the GC below is
		// free to collect it. Without it the object's lifetime is whatever the
		// optimizer decides, which is not something to assert against.
		runtime.KeepAlive(obj)
		obj = nil
		runtime.GC()

		select {
		case got := <-fired:
			if !got {
				t.Fatalf("cleanup received %v, want the arg it was registered with", got)
			}
		case <-time.After(cleanupWait):
			t.Fatal("cleanup did not run within the wait window after the object became unreachable")
		}
	})

	t.Run("Stop prevents it", func(t *testing.T) {
		fired := make(chan bool, 1)

		obj := new(tracked)
		cleanup := AddCleanup(obj, func(v bool) { fired <- v }, true)
		cleanup.Stop()
		cleanup.Stop() // Documented as safe to repeat.

		runtime.KeepAlive(obj)
		obj = nil
		runtime.GC()

		// A negative assertion, so it is bounded by a short wait rather than
		// cleanupWait: the positive case above already establishes that a live
		// registration fires well inside that window.
		select {
		case <-fired:
			t.Fatal("a stopped cleanup ran anyway")
		case <-time.After(100 * time.Millisecond):
		}
	})
}

func TestClearFinalizer(t *testing.T) {
	obj := new(tracked)
	// Should not panic, whether or not a finalizer was ever set.
	ClearFinalizer(obj)
	ClearFinalizer(obj)
}
