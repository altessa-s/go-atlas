// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// A scheduled cycle that fires on a context already canceled must not start:
// the fetch runs detached from cancellation and locks a batch, while the
// handlers run under the canceled context and attempt none of it, so the events
// would stay locked until the unlock sweeper reclaims them.
func TestDispatchCycle_CanceledContextFetchesNothing(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		run  func(*Outbox) func(context.Context) error
	}{
		{"scheduled", (*Outbox).RegisterDispatchSchedulerFunc},
		{"manual", func(ob *Outbox) func(context.Context) error { return ob.RunDispatchCycle }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := &watchableStore{batches: [][]Event{{{Id: "e1", Key: "orders.created"}}}}
			ob := New(store, noopHandler)

			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			require.NoError(t, tc.run(ob)(ctx))
			require.Zero(t, store.served.Load(), "a cycle on a canceled context must not fetch and lock a batch")
			require.Empty(t, store.updated)
		})
	}
}
