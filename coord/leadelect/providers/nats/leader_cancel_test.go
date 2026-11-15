// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"

	lenats "github.com/altessa-s/go-atlas/coord/leadelect/providers/nats"
)

// A Leader whose Start context is canceled reports the state of its provider:
// not running and not leader. Stop then pairs with that Start and allows a
// restart.
func TestLeader_StartContextCancelStopsReportingRunning(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	le, err := lenats.NewLeader(t.Context(), natstest.ConnectNATS(t, ns), "svc", "node-1")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	require.NoError(t, le.Start(ctx))
	testhelpers.WaitFor(t, 10*time.Second, le.IsLeader, "the node did not become leader")
	require.True(t, le.IsRunning())
	require.NotZero(t, le.Fence())

	cancel()
	testhelpers.WaitFor(t, 10*time.Second, func() bool { return !le.IsRunning() }, "a Leader whose election ended still reports running")
	require.False(t, le.IsLeader())
	require.Zero(t, le.Fence())

	require.NoError(t, le.Stop(t.Context()))
	require.False(t, le.IsRunning())

	require.NoError(t, le.Start(t.Context()))
	testhelpers.WaitFor(t, 10*time.Second, le.IsLeader, "the node did not become leader after a restart")
	require.True(t, le.IsRunning())
	require.NoError(t, le.Stop(t.Context()))
	require.False(t, le.IsRunning())
	require.False(t, le.IsLeader())
}
