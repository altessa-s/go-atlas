// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package msg_test

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/messaging/broker/msg"
)

func TestBackOffLargeAttempts(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		fn   msg.BackOffFunc
	}{
		{"schedule", msg.ScheduleBackOff(time.Second, time.Minute)},
		{"exponential", msg.ExponentialBackOff(time.Second, time.Minute, 0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, attempt := range []uint64{math.MaxInt64, math.MaxInt64 + 1, math.MaxInt64 + 2, math.MaxUint64} {
				require.Equal(t, time.Minute, tc.fn(attempt), "attempt %d", attempt)
			}
		})
	}
}
