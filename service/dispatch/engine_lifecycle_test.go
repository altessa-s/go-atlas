// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dispatch_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/service/dispatch"
)

// TestEngine_ShutdownWithoutStart pins that an engine which was constructed but
// never started can still be shut down.
//
// Shutdown unconditionally invokes the cancel func for the engine's lifecycle
// context. While that context was created in Start rather than in NewEngine,
// the field was nil on this path and Shutdown panicked — reachable from any
// caller that builds an engine and then bails out before starting it, which is
// exactly what a failed dependency wire-up does.
func TestEngine_ShutdownWithoutStart(t *testing.T) {
	t.Parallel()

	e, err := dispatch.NewEngine[int](&fakeSink{})
	require.NoError(t, err)

	require.NotPanics(t, func() {
		require.NoError(t, e.Shutdown(t.Context()))
	})
}

// TestEngine_WALBytesGaugeUpdatedOnFlush pins that moving the WAL size sample
// off the Submit path did not stop the gauge from being published.
//
// Stats takes the same mutex as Append, so sampling it per item doubled the
// WAL lock acquisitions on the hot path. It now runs once per flushed batch,
// which is still often enough for a size gauge — but only if it actually runs.
func TestEngine_WALBytesGaugeUpdatedOnFlush(t *testing.T) {
	t.Parallel()

	tc := testhelpers.NewTestCollector()
	sink := &fakeSink{}
	e, err := dispatch.NewEngine[int](sink,
		dispatch.WithWAL[int](t.TempDir(), intCodec{}),
		dispatch.WithCollector[int](tc),
		dispatch.WithFlushInterval[int](20*time.Millisecond))
	require.NoError(t, err)
	require.NoError(t, e.Start())

	require.True(t, e.Submit(1))

	require.Eventually(t, func() bool {
		return testhelpers.GetGaugeValue(t, tc, "test_async_wal_bytes") > 0
	}, 2*time.Second, 20*time.Millisecond, "wal_bytes was never published")

	stopCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	require.NoError(t, e.Shutdown(stopCtx))
}

// TestEngine_RetryWithJitterStillDelivers guards the jitter wiring. core/retry
// owns the distribution (TestExponential_Jitter); what can break here is the
// value handed to it — a jitter that inflates the backoff past the test's
// patience, or one that never reaches the retry loop at all.
func TestEngine_RetryWithJitterStillDelivers(t *testing.T) {
	t.Parallel()

	sink := &fakeSink{failN: 2}
	e, err := dispatch.NewEngine[int](sink,
		dispatch.WithRetryBackoff[int](5*time.Millisecond),
		dispatch.WithRetryJitter[int](1.0),
		dispatch.WithFlushInterval[int](10*time.Millisecond))
	require.NoError(t, err)
	require.NoError(t, e.Start())

	require.True(t, e.Submit(42))

	// Wait for the retry sequence to finish before shutting down. Shutdown
	// short-circuits the backoff sleep into a single last-ditch store, which
	// would consume one of the sink's scripted failures and make the outcome
	// depend on timing rather than on the retry logic under test.
	require.Eventually(t, func() bool { return len(sink.snapshot()) == 1 },
		5*time.Second, 10*time.Millisecond, "item was never delivered")

	stopCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	require.NoError(t, e.Shutdown(stopCtx))

	require.Equal(t, []int{42}, sink.snapshot(), "item must survive jittered retries")
}

// TestEngine_ConcurrentStartAndSubmit is a race-detector regression for the
// publication of the WAL handle.
//
// Submit used to gate on the started flag, which Start sets by CAS before it
// opens the WAL. That gave Submit no happens-before edge to the later
// e.log assignment: the race detector reported a data race on the field, and
// the practical consequence was worse than the report — a Submit landing in
// that window observed a nil log and silently bypassed durability, losing the
// one guarantee the WAL exists to provide. Submit now gates on ready, which is
// published after the handle is in place.
//
// Only meaningful under -race; without it this merely exercises the path.
func TestEngine_ConcurrentStartAndSubmit(t *testing.T) {
	t.Parallel()

	const attempts = 20

	for i := range attempts {
		e, err := dispatch.NewEngine[int](&fakeSink{},
			dispatch.WithWAL[int](t.TempDir(), intCodec{}))
		require.NoError(t, err)

		var (
			wg       sync.WaitGroup
			startErr error
		)
		wg.Go(func() { startErr = e.Start() })
		wg.Go(func() { e.Submit(i) })
		wg.Wait()
		require.NoError(t, startErr)

		stopCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		require.NoError(t, e.Shutdown(stopCtx))
		cancel()
	}
}
