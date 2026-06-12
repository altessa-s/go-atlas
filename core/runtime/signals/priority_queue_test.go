// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package signals

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPriorityQueueCustomPriorityOrdering proves that a handler registered
// with a custom priority that matches no predefined bucket is ordered
// relative to the predefined levels instead of being misfiled into the
// highest-priority bucket: a custom priority of 10 must run after
// PriorityLow (25) and before PriorityLowest (1), never first.
func TestPriorityQueueCustomPriorityOrdering(t *testing.T) {
	t.Parallel()

	const customLow = Priority(10)

	pq := newPriorityQueue()
	entries := makeHandlersWithPriorities([]Priority{
		PriorityLow, PriorityHighest, customLow, PriorityNormal, customLow, PriorityLowest,
	})
	for _, e := range entries {
		pq.addHandler(e)
	}

	got := pq.getHandlersInOrder()
	require.Len(t, got, len(entries))
	assertSortedDescending(t, got)
	assertStable(t, got)

	priorities := make([]Priority, len(got))
	for i, h := range got {
		priorities[i] = h.priority
	}
	require.Equal(t,
		[]Priority{PriorityHighest, PriorityNormal, PriorityLow, customLow, customLow, PriorityLowest},
		priorities)
}
