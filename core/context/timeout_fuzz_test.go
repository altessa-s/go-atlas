// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package context_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	corecontext "github.com/altessa-s/go-atlas/core/context"
)

// FuzzApplyTimeoutNeverExtendsADeadline is the deadline-propagation oracle.
//
// Every network boundary in the repo bounds itself with this helper, and the
// rule it exists to enforce is that an inner call cannot outlive its caller: a
// context that already carries a deadline keeps it, whatever timeout the callee
// would have preferred. A helper that replaced an incoming 100ms deadline with
// its own 30s default would leave a request the client already gave up on
// running for another half minute, holding a connection and a database
// transaction with it.
func FuzzApplyTimeoutNeverExtendsADeadline(f *testing.F) {
	f.Add(int64(0), int64(time.Second))
	f.Add(int64(time.Millisecond), int64(time.Hour))
	f.Add(int64(time.Hour), int64(time.Millisecond))
	f.Add(int64(-1), int64(time.Second))
	f.Add(int64(time.Second), int64(0))

	f.Fuzz(func(t *testing.T, parentNanos, timeoutNanos int64) {
		const bound = int64(time.Hour)
		if parentNanos <= 0 || parentNanos > bound || timeoutNanos < 0 || timeoutNanos > bound {
			t.Skip("durations outside the plausible range say nothing about the rule")
		}

		parent, cancelParent := context.WithTimeout(t.Context(), time.Duration(parentNanos))
		defer cancelParent()

		parentDeadline, ok := parent.Deadline()
		require.True(t, ok)

		derived, cancel := corecontext.ApplyTimeout(parent, time.Duration(timeoutNanos))
		defer cancel()

		derivedDeadline, ok := derived.Deadline()
		require.True(t, ok, "a context that had a deadline lost it")
		require.False(t, derivedDeadline.After(parentDeadline),
			"the derived deadline outlives its parent: parent=%v timeout=%v",
			time.Duration(parentNanos), time.Duration(timeoutNanos))
	})
}

// FuzzApplyTimeoutBoundsADeadlinelessContext pins the other half: a context
// with no deadline gets one, unless the caller asked for none.
//
// This is the case that turns an unbounded call into a bounded one. A helper
// that silently returned the context unchanged would leave every call site
// believing it had a timeout while none was ever installed — the failure only
// shows up when a dependency hangs, which is exactly when it matters.
func FuzzApplyTimeoutBoundsADeadlinelessContext(f *testing.F) {
	f.Add(int64(time.Second))
	f.Add(int64(0))
	f.Add(int64(-1))
	f.Add(int64(time.Nanosecond))

	f.Fuzz(func(t *testing.T, timeoutNanos int64) {
		if timeoutNanos < 0 || timeoutNanos > int64(time.Hour) {
			t.Skip("durations outside the plausible range say nothing about the rule")
		}

		// context.Background has no deadline, unlike t.Context.
		derived, cancel := corecontext.ApplyTimeout(context.Background(), time.Duration(timeoutNanos))
		defer cancel()

		_, hasDeadline := derived.Deadline()
		require.Equal(t, timeoutNanos != 0, hasDeadline,
			"timeout %v produced hasDeadline=%v", time.Duration(timeoutNanos), hasDeadline)
	})
}

// FuzzApplyTimeoutPreservesCancellation pins that the derived context still
// belongs to its parent: cancelling upstream cancels it.
//
// A helper that built its context from Background instead of the caller's would
// satisfy both targets above and still detach every call from the request that
// made it — the classic leak where a canceled request keeps working.
func FuzzApplyTimeoutPreservesCancellation(f *testing.F) {
	f.Add(int64(time.Second))
	f.Add(int64(0))

	f.Fuzz(func(t *testing.T, timeoutNanos int64) {
		if timeoutNanos < 0 || timeoutNanos > int64(time.Hour) {
			t.Skip("durations outside the plausible range say nothing about the rule")
		}

		parent, cancelParent := context.WithCancel(context.Background())
		derived, cancel := corecontext.ApplyTimeout(parent, time.Duration(timeoutNanos))
		defer cancel()

		cancelParent()

		select {
		case <-derived.Done():
		case <-time.After(time.Second):
			t.Fatalf("cancelling the parent did not cancel the derived context (timeout %v)",
				time.Duration(timeoutNanos))
		}
		require.Error(t, derived.Err())

		// Which error it reports depends on what happened first, and a timeout
		// short enough to expire between the two statements above legitimately
		// reports DeadlineExceeded. Only a timeout that cannot have fired yet
		// pins the cause to the parent.
		if timeoutNanos == 0 || timeoutNanos >= int64(time.Second) {
			require.ErrorIs(t, derived.Err(), context.Canceled,
				"the derived context ended for a reason other than its parent (timeout %v)",
				time.Duration(timeoutNanos))
		}
	})
}
