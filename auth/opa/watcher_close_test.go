// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa

import (
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"
)

// Hold an admitted delivery between taking the subscriber snapshot and
// sending. Stop must signal cancellation, then join the delivery.
func TestWatchCloseJoinsAdmittedDelivery(t *testing.T) {
	t.Parallel()
	for _, managerClose := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			w := newWatchManager()
			r := w.subscribe(WatchOptions{BufferSize: 1, BufferOverflowPolicy: OverflowPolicyBlock})
			var s *subscriber
			for sub := range w.subscribers {
				s = sub
			}
			s.inFlight.Add(1)
			release := make(chan struct{})
			go func() {
				<-release
				defer s.inFlight.Done()
				w.sendEvent(s, PolicyEvent{})
			}()
			closed := make(chan struct{})
			go func() {
				if managerClose {
					w.close(func() {})
				} else {
					r.Stop()
				}
				close(closed)
			}()
			<-r.Done
			synctest.Wait()
			select {
			case <-closed:
				t.Error("returned before the admitted delivery finished")
			default:
			}
			close(release)
			<-closed
			_, open := <-r.Events
			require.False(t, open)
		})
	}
}
