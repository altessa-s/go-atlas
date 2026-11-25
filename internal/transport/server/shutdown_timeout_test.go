// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package server

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/transport/timeouts"
)

func TestGracefulShutdownDeadline(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name               string
		configured, caller time.Duration
	}{
		{"configured limit", time.Minute, time.Hour},
		{"caller deadline", time.Hour, time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := NewBaseServer(WithListener(&lifecycleListener{}), WithTimeouts(timeouts.Lifecycle{ShutdownGraceful: tc.configured, ShutdownWarning: time.Second}))
			require.NoError(t, srv.Start("test", func(net.Listener, chan<- error) {}))
			ctx, cancel := context.WithTimeout(t.Context(), tc.caller)
			defer cancel()
			before := time.Now()
			observed := make(chan time.Time, 1)
			require.NoError(t, srv.GracefulShutdown(ctx, func(ctx context.Context) error {
				deadline, _ := ctx.Deadline()
				observed <- deadline
				return nil
			}))
			deadline := <-observed
			callerDeadline, _ := ctx.Deadline()
			if tc.caller < tc.configured {
				require.Equal(t, callerDeadline, deadline)
			} else {
				require.False(t, deadline.Before(before.Add(tc.configured)))
				require.False(t, deadline.After(time.Now().Add(tc.configured)), "configured shutdown limit did not reach the protocol")
			}
		})
	}
}

func TestPartialTimeoutsUseShutdownDefaults(t *testing.T) {
	t.Parallel()
	srv := NewBaseServer(WithTimeouts(timeouts.Lifecycle{}))
	limits := srv.Timeouts()
	require.Zero(t, limits.StartupVerification)
	require.Equal(t, timeouts.DefaultShutdownGraceful, limits.ShutdownGraceful)
	require.Equal(t, timeouts.DefaultShutdownWarning, limits.ShutdownWarning)
}
