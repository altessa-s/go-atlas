// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package leadelectit_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/leadelect"
	"github.com/altessa-s/go-atlas/tests/integration/leadelectit"
)

// TestElection_SingleLeaderAmongPeers asserts the property the package exists
// to provide: with several electors competing for one key through a real
// broker, at most one of them believes it leads at any sampled instant, and it
// keeps believing it across many lease renewals.
func TestElection_SingleLeaderAmongPeers(t *testing.T) {
	t.Parallel()

	c := newCluster(t, 5)
	elected := c.awaitLeader(t, 10*time.Second)

	obs := leadelectit.NewObserver(samplingInterval, c.electors()...)
	obs.Start(t.Context())

	// Span several renewal intervals so a renewal that silently fails shows up
	// as a lost or handed-over term rather than passing unnoticed.
	time.Sleep(4 * electionTTL)
	obs.Stop()

	require.Empty(t, obs.Overlaps(), "two nodes claimed leadership at once\n%s", obs.Timeline())
	require.Equal(t, []string{elected.id}, obs.Leaders(),
		"leadership should not move while the holder is healthy\n%s", obs.Timeline())
	require.Empty(t, obs.FenceRegression(), obs.Timeline())
}

// TestElection_FenceAdvancesWithRenewals asserts that renewing the lease
// advances the fencing token. A token that stood still across renewals would
// still be monotonic, and would still be useless: a downstream store could not
// tell a fresh write from a replayed one.
func TestElection_FenceAdvancesWithRenewals(t *testing.T) {
	t.Parallel()

	c := newCluster(t, 1)
	elected := c.awaitLeader(t, 10*time.Second)

	first := elected.elector.Fence()
	require.NotZero(t, first, "a fresh leader must report a non-zero fencing token")

	require.Eventually(t, func() bool { return elected.elector.Fence() > first },
		4*electionTTL, samplingInterval, "fencing token never advanced past %d across renewals", first)
}

// TestElection_HandoverOnGracefulStop asserts that a leader which resigns
// releases the key immediately, so a successor takes over without waiting for
// the key to age out, and opens its term above the resigning leader's token.
func TestElection_HandoverOnGracefulStop(t *testing.T) {
	t.Parallel()

	c := newCluster(t, 3)
	elected := c.awaitLeader(t, 10*time.Second)

	obs := leadelectit.NewObserver(samplingInterval, c.electors()...)
	obs.Start(t.Context())
	defer obs.Stop()

	handedOver := elected.elector.Fence()
	require.NoError(t, elected.elector.Stop(t.Context()))

	// A resign deletes the key, and the survivors watch it, so the handover
	// must not need the key-expiry path.
	start := time.Now()
	successor := c.awaitLeaderOtherThan(t, elected.id, keyExpiry)
	require.Less(t, time.Since(start), keyExpiry,
		"a resigned lease should hand over via the watcher, not by waiting for expiry")

	require.Greater(t, successor.elector.Fence(), handedOver,
		"the successor must open its term above the resigning leader's token")

	// Let the new term run long enough for the Observer to record it; the
	// handover itself is far quicker than the sampling interval.
	time.Sleep(10 * samplingInterval)
	obs.Stop()
	require.Empty(t, obs.Overlaps(), "two nodes claimed leadership during the handover\n%s", obs.Timeline())
	require.Empty(t, obs.FenceRegression(), obs.Timeline())
}

// TestElection_FailoverAfterAbruptLoss asserts the guarantee behind "a lock
// always has a TTL": a holder that disappears without resigning still releases
// the lease, because the server ages the key out.
//
// This is the path no unit test reaches. It also pins that the failover is
// bounded by the bucket's key expiry rather than by the election TTL — those
// are different durations, and only the former governs here.
func TestElection_FailoverAfterAbruptLoss(t *testing.T) {
	t.Parallel()

	c := newCluster(t, 3)
	elected := c.awaitLeader(t, 10*time.Second)
	fenceBefore := elected.elector.Fence()

	obs := leadelectit.NewObserver(samplingInterval, c.electors()...)
	obs.Start(t.Context())
	defer obs.Stop()

	// No Stop: the lease is abandoned, exactly as it would be by a SIGKILL.
	elected.kill()

	successor := c.awaitLeaderOtherThan(t, elected.id, keyExpiry+5*time.Second)
	require.Greater(t, successor.elector.Fence(), fenceBefore,
		"the successor must fence out the abandoned term")

	obs.Stop()
	require.Empty(t, obs.Overlaps(),
		"the abandoned leader still claimed leadership while its successor ran\n%s", obs.Timeline())
	require.Empty(t, obs.FenceRegression(), obs.Timeline())
}

// TestElection_PartitionedLeaderSelfDemotes asserts the split-brain guard: a
// node cut off from the broker stops claiming leadership on its own, within the
// lease, and nobody has to tell it. Its fencing token drops to zero with it, so
// a downstream store rejects its late writes even if the application has not
// noticed yet.
//
// The bound asserted here is the lease. In practice the demotion is usually
// quicker, because a severed connection makes the next renewal fail outright
// rather than merely time out; the lease is the guarantee that holds when it
// does not, so that is what the deadline is set to.
func TestElection_PartitionedLeaderSelfDemotes(t *testing.T) {
	t.Parallel()

	c := newCluster(t, 1)
	elected := c.awaitLeader(t, 10*time.Second)
	require.NotZero(t, elected.elector.Fence(), "precondition: leader reports a fencing token")

	elected.kill()

	require.Eventually(t, func() bool { return !elected.elector.IsLeader() },
		electionTTL+2*time.Second, samplingInterval,
		"a partitioned leader kept claiming leadership past its lease")

	require.Zero(t, elected.elector.Fence(),
		"a demoted leader must not hand out a token a downstream store would accept")
}

// TestElection_CallbacksFireOnTransitions asserts the callback contract end to
// end: the elected node is told it became leader, and the node that takes over
// after it resigns is told in turn.
func TestElection_CallbacksFireOnTransitions(t *testing.T) {
	t.Parallel()

	// Buffered and registered before Start: leadership is acquired within
	// milliseconds, so a callback hooked up afterwards would miss the very
	// transition under test.
	became := make(chan string, 8)
	c := newCluster(t, 2, func(le *leadelect.Leader) {
		le.RegisterOnBecomesLeader(func(_ context.Context, e leadelect.LeaderElector) {
			became <- e.NodeId()
		})
	})

	elected := c.awaitLeader(t, 10*time.Second)

	select {
	case got := <-became:
		require.Equal(t, elected.id, got, "the became-leader callback fired on the wrong node")
	case <-time.After(5 * time.Second):
		t.Fatal("no became-leader callback fired for the elected node")
	}

	require.NoError(t, elected.elector.Stop(t.Context()))
	successor := c.awaitLeaderOtherThan(t, elected.id, keyExpiry)

	select {
	case got := <-became:
		require.Equal(t, successor.id, got, "the successor's callback fired on the wrong node")
	case <-time.After(5 * time.Second):
		t.Fatal("no became-leader callback fired for the successor")
	}
}

// TestElection_FenceMonotonicAcrossRepeatedFailovers drives leadership through
// several hands in a row and asserts the fencing token never stalls or moves
// backwards across the whole sequence. One handover can look monotonic by
// luck; a chain of them is the property a downstream store actually depends on.
func TestElection_FenceMonotonicAcrossRepeatedFailovers(t *testing.T) {
	t.Parallel()

	const (
		handovers = 3

		// Each term is held briefly before it is handed on. A resign-driven
		// handover completes in tens of milliseconds, so without this the whole
		// sequence would finish inside a couple of sampling rounds and the
		// Observer would have nothing to say about it.
		termDwell = 10 * samplingInterval
	)

	c := newCluster(t, handovers+1)

	obs := leadelectit.NewObserver(samplingInterval, c.electors()...)
	obs.Start(t.Context())
	defer obs.Stop()

	// The fences are read directly rather than taken from the recording: the
	// assertion is about every term that happened, not only the ones a poller
	// happened to catch.
	var (
		fences  []uint64
		holders []string
		stopped string
	)
	for range handovers {
		elected := c.awaitLeaderOtherThan(t, stopped, 10*time.Second)
		time.Sleep(termDwell)

		fences = append(fences, elected.elector.Fence())
		holders = append(holders, elected.id)

		require.NoError(t, elected.elector.Stop(t.Context()))
		stopped = elected.id
	}

	last := c.awaitLeaderOtherThan(t, stopped, 10*time.Second)
	time.Sleep(termDwell)
	fences = append(fences, last.elector.Fence())
	holders = append(holders, last.id)

	obs.Stop()

	for i, f := range fences {
		require.NotZero(t, f, "term %d (%s) reported no fencing token", i, holders[i])
		if i == 0 {
			continue
		}
		require.Greater(t, f, fences[i-1],
			"term %d (%s) opened at fence %d, not above term %d (%s) at %d\n%s",
			i, holders[i], f, i-1, holders[i-1], fences[i-1], obs.Timeline())
	}

	require.Empty(t, obs.Overlaps(), "two nodes claimed leadership during a handover\n%s", obs.Timeline())
	require.Empty(t, obs.FenceRegression(), obs.Timeline())
	require.GreaterOrEqual(t, len(obs.Terms()), handovers+1,
		"expected at least %d terms across %d handovers\n%s", handovers+1, handovers, obs.Timeline())
}
