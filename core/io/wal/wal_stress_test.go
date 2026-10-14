// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build stress

package wal_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/io/wal"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// TestStressWAL runs group-committed Append from many producers against a
// ReadPending/Ack consumer while segments roll and Sync, Stats and Faulted
// run concurrently. Every appended record must be delivered exactly once,
// Pending must return to zero, and recovery after Close must return every
// unacked record and only records that share a segment with one.
func TestStressWAL(t *testing.T) {
	duration := testhelpers.StressDuration(t)
	dir := t.TempDir()
	w, recovered, err := wal.Open(dir, wal.WithMaxSegmentBytes(4<<10), wal.WithFsyncInterval(time.Millisecond))
	require.NoError(t, err)
	require.Empty(t, recovered)

	const producers = 16
	var (
		stop      atomic.Bool
		nextID    atomic.Uint64
		appendMu  sync.Mutex
		appended  = make(map[uint64]wal.Offset)
		delivered = make(map[uint64]struct{}) // consumer-owned until consumerDone
		syncs     atomic.Int64
		wg        sync.WaitGroup
	)
	nextID.Store(1)

	for range producers {
		wg.Go(func() {
			for !stop.Load() {
				id := nextID.Add(1) - 1
				off, err := w.Append(recordPayload(id))
				if err != nil {
					t.Errorf("Append(%d): %v", id, err)
					stop.Store(true)
					return
				}
				appendMu.Lock()
				appended[id] = off
				appendMu.Unlock()
			}
		})
	}
	wg.Go(func() {
		for !stop.Load() {
			if err := w.Sync(); err != nil {
				t.Errorf("Sync: %v", err)
				stop.Store(true)
				return
			}
			syncs.Add(1)
			if err := w.Faulted(); err != nil {
				t.Errorf("Faulted: %v", err)
				stop.Store(true)
				return
			}
			if s := w.Stats(); s.Pending < 0 || s.TotalBytes < 0 || s.Segments < 1 {
				t.Errorf("Stats out of range: %+v", s)
				stop.Store(true)
				return
			}
			time.Sleep(time.Millisecond)
		}
	})

	consume := func() (int, bool) {
		recs, err := w.ReadPending(128, nil)
		if err != nil {
			t.Errorf("ReadPending: %v", err)
			return 0, false
		}
		for _, r := range recs {
			id, ok := recordID(r.Payload)
			if !ok {
				t.Errorf("ReadPending returned a corrupt payload at %+v", r.Offset)
				return 0, false
			}
			if _, dup := delivered[id]; dup {
				t.Errorf("record %d at %+v delivered twice", id, r.Offset)
				return 0, false
			}
			delivered[id] = struct{}{}
			w.Ack(r.Offset)
		}
		return len(recs), true
	}
	consumerDone := make(chan struct{})
	go func() {
		defer close(consumerDone)
		for !stop.Load() {
			if _, ok := consume(); !ok {
				stop.Store(true)
				return
			}
		}
	}()

	time.Sleep(duration)
	stop.Store(true)
	wg.Wait()
	<-consumerDone
	require.False(t, t.Failed(), "invariant violated during the run")

	for {
		n, ok := consume()
		require.True(t, ok)
		if n == 0 {
			break
		}
	}
	require.Len(t, delivered, len(appended), "every appended record must be delivered exactly once")
	for id := range appended {
		if _, ok := delivered[id]; !ok {
			require.FailNow(t, "record appended but never delivered", "id %d", id)
		}
	}
	require.Zero(t, w.Stats().Pending, "Pending must return to 0 once everything is acked")
	t.Logf("appended=%d syncs=%d segments=%d", len(appended), syncs.Load(), w.Stats().Segments)

	// Leave a tail of unacked records spread over several segments.
	tail := make(map[uint64]wal.Offset)
	unacked := make(map[uint64]wal.Offset)
	for i := range uint64(2000) {
		id := nextID.Add(1) - 1
		off, err := w.Append(recordPayload(id))
		require.NoError(t, err)
		tail[id] = off
		if i%3 == 0 {
			w.Ack(off)
		} else {
			unacked[id] = off
		}
	}
	require.Equal(t, int64(len(unacked)), w.Stats().Pending)
	require.NoError(t, w.Close())

	w2, recovered, err := wal.Open(dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = w2.Close() })
	liveSegments := make(map[uint64]struct{})
	for _, off := range unacked {
		liveSegments[off.SegmentID] = struct{}{}
	}
	seen := make(map[uint64]struct{}, len(recovered))
	for _, r := range recovered {
		id, ok := recordID(r.Payload)
		require.True(t, ok, "recovery returned a corrupt payload at %+v", r.Offset)
		off, known := tail[id]
		if !known {
			off, known = appended[id]
		}
		require.True(t, known, "recovery returned record %d, which was never appended", id)
		require.Equal(t, off, r.Offset, "record %d recovered at a different offset", id)
		// At-least-once: an acked record may come back, but only from a
		// segment that still holds an unacked one.
		_, live := liveSegments[r.Offset.SegmentID]
		require.True(t, live, "acked record %d recovered from segment %d, which holds no unacked record", id, r.Offset.SegmentID)
		seen[id] = struct{}{}
	}
	for id := range unacked {
		_, ok := seen[id]
		require.True(t, ok, "unacked record %d lost across Close", id)
	}
}
