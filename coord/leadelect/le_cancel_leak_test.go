// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package leadelect_test

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/leadelect"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"

	lenats "github.com/altessa-s/go-atlas/coord/leadelect/providers/nats"
)

// An election ended by canceling the context passed to Start, with no Stop
// and no restart after it, leaves none of its goroutines behind.
//
// Serial: it scans every goroutine in the process, so a parallel test running
// its own election would make the assertion meaningless.
func TestLeader_StartContextCancelWithoutStopLeaksNoGoroutine(t *testing.T) {
	electionGoroutines := func() []string {
		buf := make([]byte, 1<<20)
		stacks := string(buf[:runtime.Stack(buf, true)])
		var alive []string
		for _, fn := range []string{
			"leadelect.(*Leader).dispatch",
			"nats.(*Provider).notificationHandler",
			"nats.(*Provider).camping",
		} {
			if strings.Contains(stacks, fn) {
				alive = append(alive, fn)
			}
		}
		return alive
	}

	ns := natstest.StartNATSServer(t)
	prov, err := lenats.New(t.Context(), natstest.ConnectNATS(t, ns), lenats.WithBucket("cancel-leak"))
	require.NoError(t, err)

	le := leadelect.New(prov, "cancel-leak", "node-1", leadelect.WithTTL(2*time.Second))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	require.NoError(t, le.Start(ctx))
	testhelpers.WaitFor(t, 10*time.Second, le.IsLeader, "the node did not become leader")
	require.Len(t, electionGoroutines(), 3, "the election goroutines should be running while the election is active")

	cancel()
	testhelpers.WaitFor(t, 10*time.Second, func() bool { return !le.IsRunning() }, "the election did not end with its context")
	require.Eventually(t, func() bool { return len(electionGoroutines()) == 0 }, 10*time.Second, 10*time.Millisecond,
		"goroutines of an election ended by its context are still running: %v", electionGoroutines())
}
