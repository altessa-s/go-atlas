// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestWatcher_NotifyCloseRace pins that a notify racing with close is
// synchronized: under -race, a send and a close on the same channel must never
// overlap, and a notify after close reports false without touching the channel.
func TestWatcher_NotifyCloseRace(t *testing.T) {
	t.Parallel()

	for range 200 {
		w := newWatcher(1, StatusServing)
		w.remove = w.close

		var wg sync.WaitGroup
		for range 4 {
			wg.Go(func() {
				for range 8 {
					w.notify(StatusNotServing)
				}
			})
		}
		wg.Go(func() { w.stop(false) })
		wg.Wait()

		require.False(t, w.notify(StatusServing), "notify after close must not send")
	}
}
