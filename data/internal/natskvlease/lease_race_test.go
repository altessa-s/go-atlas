// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natskvlease

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLease_UpdateValue_RaceFree is the regression guard for the
// atomic-pointer fix. UpdateValue is invoked from arbitrary caller
// goroutines (e.g. dlock OnRenewed callbacks), while the renew
// goroutine reads the same field on every cycle. Before the fix
// l.config.Value = val concurrently with a read was a Go data race;
// the race detector flagged it under -race.
//
// The test must run cleanly under -race for the fix to be considered
// effective. We do NOT need a live NATS connection — only the
// in-process atomic.Pointer logic is exercised.
func TestLease_UpdateValue_RaceFree(t *testing.T) {
	// Construct with nil kv: NewKVOps stores the nil but is never
	// dereferenced because UpdateValue/currentValue do not touch the
	// kv. If a future change makes them touch it, the test will panic
	// loudly — caught immediately.
	l := NewLease(nil, LeaseConfig{
		Key:   "lease",
		Value: []byte("initial"),
	})

	const writers = 8
	const reads = 10_000

	var wg sync.WaitGroup
	wg.Add(writers + 1)

	// Reader: simulates the renew goroutine touching currentValue on
	// every cycle.
	go func() {
		defer wg.Done()
		for range reads {
			_ = l.currentValue()
		}
	}()

	// Writers: simulate concurrent UpdateValue callbacks.
	for i := range writers {
		go func(id int) {
			defer wg.Done()
			for j := range reads {
				l.UpdateValue([]byte{byte(id), byte(j)})
			}
		}(i)
	}

	wg.Wait()

	// Sanity: after the storm, currentValue is one of the writers'
	// last-written values (or the initial value if a writer somehow
	// never ran). Just assert it's non-nil; the meaningful assertion
	// is that -race did not complain.
	require.NotNil(t, l.currentValue(),
		"currentValue must always return a usable buffer — even mid-swap")
}

// TestLease_UpdateValue_DefensiveCopy proves that mutating the caller's
// slice after UpdateValue does NOT corrupt the lease's stored value.
// Without the defensive copy, the renew goroutine could read partial
// updates as the caller mutated the slice in place.
func TestLease_UpdateValue_DefensiveCopy(t *testing.T) {
	l := NewLease(nil, LeaseConfig{Key: "lease", Value: []byte("initial")})

	buf := []byte("v1")
	l.UpdateValue(buf)

	// Mutate the caller's buffer. If the lease retained a reference to
	// it, currentValue would now return "v2".
	buf[0] = 'X'
	buf[1] = 'X'

	require.Equal(t, "v1", string(l.currentValue()),
		"UpdateValue must take a defensive copy so post-call mutations by the caller are invisible to the renew goroutine")
}

// TestLease_NewLease_DefensiveCopyOfInitialValue mirrors the
// defensive-copy check for the NewLease seed path: mutating the slice
// originally passed in LeaseConfig.Value must not change what
// currentValue returns later.
func TestLease_NewLease_DefensiveCopyOfInitialValue(t *testing.T) {
	initial := []byte("v1")
	l := NewLease(nil, LeaseConfig{Key: "lease", Value: initial})

	initial[0] = 'X'
	initial[1] = 'X'

	require.Equal(t, "v1", string(l.currentValue()),
		"NewLease must take a defensive copy of cfg.Value so caller mutations cannot race with the renew goroutine")
}
