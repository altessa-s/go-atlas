// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build stress

package dispatch_test

import (
	"context"
	"errors"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/io/wal"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/service/dispatch"
)

var errChaos = errors.New("chaos sink failure")

// chaosSink fails some batches before storing them and, rarely, after: the
// second kind is the ambiguous outcome that makes at-least-once delivery
// produce a duplicate. It enforces the contract on every store: an item may
// be delivered again only as often as it was part of an ambiguous batch or
// replayed from the WAL by a restart.
type chaosSink struct {
	t       testing.TB
	enabled atomic.Bool

	mu        sync.Mutex
	delivered map[int]int
	allowance map[int]int // ambiguous failures plus replays, per item
	failures  int
	dups      int
}

// allow permits one more delivery of id.
func (s *chaosSink) allow(id int) {
	s.mu.Lock()
	s.allowance[id]++
	s.mu.Unlock()
}

func (s *chaosSink) StoreBatch(_ context.Context, items []int) error {
	roll := rand.IntN(100) //nolint:gosec // test chaos
	if s.enabled.Load() && roll < 10 {
		s.mu.Lock()
		s.failures++
		s.mu.Unlock()
		return errChaos
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range items {
		if n := s.delivered[id]; n > s.allowance[id] {
			s.t.Errorf("item %d delivered %d times with %d ambiguous failures and replays: duplicate beyond the at-least-once contract",
				id, n+1, s.allowance[id])
		} else if n > 0 {
			s.dups++
		}
		s.delivered[id]++
	}
	if s.enabled.Load() && roll < 13 {
		for _, id := range items {
			s.allowance[id]++
		}
		return errChaos
	}
	return nil
}

// TestStressDispatchWAL restarts a WAL-backed engine in epochs. In each epoch
// producers enqueue while workers deliver to a sink that fails transiently,
// and Shutdown runs while producers are still submitting. A final epoch with a
// healthy sink drains the backlog. No accepted item may be lost, and duplicates
// stay within the contract enforced by chaosSink.
func TestStressDispatchWAL(t *testing.T) {
	deadline := time.Now().Add(testhelpers.StressDuration(t))
	dir := t.TempDir()
	sink := &chaosSink{t: t, delivered: make(map[int]int), allowance: make(map[int]int)}
	sink.enabled.Store(true)

	var (
		nextID   atomic.Int64
		acceptMu sync.Mutex
		accepted = make(map[int]struct{})
		epochs   int
	)
	// Acks live in memory, so a restart replays every record of a segment
	// that still held an unacked one, acked or not. Read what the next Start
	// will replay and allow those deliveries.
	newEngine := func() *dispatch.Engine[int] {
		w, recovered, err := wal.Open(dir)
		require.NoError(t, err)
		for _, r := range recovered {
			id, err := intCodec{}.Decode(r.Payload)
			require.NoError(t, err)
			sink.allow(id)
		}
		require.NoError(t, w.Close())
		e, err := dispatch.NewEngine[int](sink,
			dispatch.WithWorkers[int](4),
			dispatch.WithBatchSize[int](16),
			dispatch.WithFlushInterval[int](5*time.Millisecond),
			dispatch.WithRetryAttempts[int](2),
			dispatch.WithRetryBackoff[int](time.Millisecond),
			dispatch.WithRetryMaxBackoff[int](5*time.Millisecond),
			dispatch.WithWAL[int](dir, intCodec{}, wal.WithMaxSegmentBytes(2<<10), wal.WithFsyncInterval(time.Millisecond)),
		)
		require.NoError(t, err)
		require.NoError(t, e.Start())
		return e
	}

	for time.Now().Before(deadline) && !t.Failed() {
		epochs++
		e := newEngine()
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				for {
					id := int(nextID.Add(1))
					if _, err := e.Enqueue(id); err != nil {
						if !errors.Is(err, dispatch.ErrEngineClosed) {
							t.Errorf("Enqueue(%d): %v", id, err)
						}
						return
					}
					acceptMu.Lock()
					accepted[id] = struct{}{}
					acceptMu.Unlock()
				}
			})
		}
		time.Sleep(time.Duration(200+rand.IntN(800)) * time.Millisecond) //nolint:gosec // test jitter
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		err := e.Shutdown(ctx)
		cancel()
		wg.Wait()
		if err != nil && !errors.Is(err, dispatch.ErrBacklog) && !errors.Is(err, errChaos) {
			require.NoError(t, err, "epoch %d: unexpected Shutdown error", epochs)
		}
		require.Zero(t, e.Dropped(), "epoch %d: a WAL-backed engine dropped items", epochs)
	}
	require.False(t, t.Failed(), "invariant violated during the run")

	sink.enabled.Store(false)
	e := newEngine()
	testhelpers.WaitFor(t, time.Minute, func() bool { return e.WAL().Stats().Pending == 0 }, "backlog did not drain with a healthy sink")
	require.NoError(t, e.Shutdown(t.Context()))

	sink.mu.Lock()
	defer sink.mu.Unlock()
	for id := range accepted {
		if sink.delivered[id] == 0 {
			require.FailNow(t, "accepted item lost", "item %d was accepted but never delivered", id)
		}
	}
	for id := range sink.delivered {
		if _, ok := accepted[id]; !ok {
			require.FailNow(t, "rejected item delivered", "item %d was delivered but Enqueue never accepted it", id)
		}
	}
	t.Logf("epochs=%d accepted=%d failures=%d duplicates=%d", epochs, len(accepted), sink.failures, sink.dups)
}
