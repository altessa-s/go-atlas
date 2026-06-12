// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package wal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// activeUnacked returns the number of unacked records in the active segment.
// Defined here so the production file does not carry test-only scaffolding.
func (w *WAL) activeUnacked() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.active == nil {
		return 0
	}
	return w.active.unacked
}

// newTestWAL opens a WAL in a fresh temp directory with a fast fsync cadence
// and returns both the WAL and its directory. Close is wired via t.Cleanup.
func newTestWAL(tb testing.TB, extraOpts ...Option) (*WAL, string) {
	tb.Helper()
	dir := tb.TempDir()
	opts := append([]Option{
		WithMaxSegmentBytes(1 << 20),
		WithFsyncInterval(2 * time.Millisecond),
	}, extraOpts...)
	w, recovered, err := Open(dir, opts...)
	require.NoError(tb, err)
	require.Empty(tb, recovered)
	tb.Cleanup(func() { _ = w.Close() })
	return w, dir
}

func TestWAL_Close_Concurrent_NoPanic(t *testing.T) {
	t.Parallel()
	w, _ := newTestWAL(t)

	const goroutines = 16
	start := make(chan struct{})
	errs := make(chan error, goroutines)
	var wg sync.WaitGroup
	for range goroutines {
		wg.Go(func() {
			<-start
			errs <- w.Close()
		})
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
}

func TestWAL_Append_EmptyPayload_IsRejected(t *testing.T) {
	t.Parallel()
	w, dir := newTestWAL(t)

	_, err := w.Append(nil)
	require.ErrorIs(t, err, ErrEmptyPayload)
	_, err = w.Append([]byte{})
	require.ErrorIs(t, err, ErrEmptyPayload)

	// A record written after the rejected appends must survive recovery:
	// nothing with a zero length may reach the segment file.
	_, err = w.Append([]byte("alive"))
	require.NoError(t, err)
	require.NoError(t, w.Close())

	w2, recovered, err := Open(dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = w2.Close() })
	require.Len(t, recovered, 1)
	require.Equal(t, []byte("alive"), recovered[0].Payload)
}

func TestWAL_Append_PayloadTooLarge_IsRejected(t *testing.T) {
	t.Parallel()
	w, _ := newTestWAL(t)

	_, err := w.Append(make([]byte, maxRecordBytes+1))
	require.ErrorIs(t, err, ErrPayloadTooLarge)
}

func TestOpen_EmptyDir_ReturnsError(t *testing.T) {
	t.Parallel()
	w, recovered, err := Open("")
	require.Error(t, err)
	require.Nil(t, w)
	require.Nil(t, recovered)
}

func TestOffset_IsZero(t *testing.T) {
	t.Parallel()
	require.True(t, Offset{}.IsZero())
	require.False(t, Offset{SegmentID: 1}.IsZero())
	require.False(t, Offset{Position: 1}.IsZero())
	require.False(t, Offset{SegmentID: 1, Position: 1}.IsZero())
}

func TestWAL_AppendAckClose_LeavesEmptyDir(t *testing.T) {
	t.Parallel()
	w, dir := newTestWAL(t)

	var offsets []Offset
	for range 10 {
		off, err := w.Append([]byte("hello"))
		require.NoError(t, err)
		require.False(t, off.IsZero())
		offsets = append(offsets, off)
	}
	require.Equal(t, int64(10), w.activeUnacked())

	for _, off := range offsets {
		w.Ack(off)
	}
	require.Equal(t, int64(0), w.activeUnacked())

	require.NoError(t, w.Close())

	matches, _ := filepath.Glob(filepath.Join(dir, "*.wal"))
	require.Empty(t, matches, "wal dir must be empty after clean shutdown")
}

func TestWAL_Ack_ZeroOffset_IsNoop(t *testing.T) {
	t.Parallel()
	w, _ := newTestWAL(t)

	_, err := w.Append([]byte("seed"))
	require.NoError(t, err)
	require.Equal(t, int64(1), w.activeUnacked())

	w.Ack(Offset{}) // zero offset must not decrement
	require.Equal(t, int64(1), w.activeUnacked())
}

func TestWAL_Sync_NoError(t *testing.T) {
	t.Parallel()
	w, _ := newTestWAL(t, WithFsyncInterval(time.Hour)) // disable background

	_, err := w.Append([]byte("payload"))
	require.NoError(t, err)
	require.NoError(t, w.Sync())
}

func TestWAL_Stats_ReflectsAppendedBytes(t *testing.T) {
	t.Parallel()
	w, _ := newTestWAL(t)

	require.Equal(t, Stats{Segments: 1, TotalBytes: 0}, w.Stats())

	payload := []byte("hello") // 5 bytes + 8-byte header = 13 bytes
	_, err := w.Append(payload)
	require.NoError(t, err)

	stats := w.Stats()
	require.Equal(t, 1, stats.Segments)
	require.Equal(t, int64(recordHeaderSize+len(payload)), stats.TotalBytes)
}

func TestWAL_SegmentRoll_CreatesMultipleSegments(t *testing.T) {
	t.Parallel()
	// Segment size chosen so three ~100-byte records fit before rolling.
	const segBytes = int64(recordHeaderSize+100) * 3
	w, dir := newTestWAL(t, WithMaxSegmentBytes(segBytes))

	payload := make([]byte, 100)
	for range 10 {
		_, err := w.Append(payload)
		require.NoError(t, err)
	}

	stats := w.Stats()
	require.GreaterOrEqual(t, stats.Segments, 2, "expected multiple segments after roll")

	matches, _ := filepath.Glob(filepath.Join(dir, "*.wal"))
	require.GreaterOrEqual(t, len(matches), 2, "expected multiple segment files on disk")
}

func TestWAL_Recovery_ReplaysUnackedRecords(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// First session: append but never ack, so records survive Close.
	w1, recovered1, err := Open(dir, WithMaxSegmentBytes(128))
	require.NoError(t, err)
	require.Empty(t, recovered1)

	payloads := [][]byte{
		[]byte("alpha"),
		[]byte("bravo"),
		[]byte("charlie"),
		[]byte("delta"),
	}
	for _, p := range payloads {
		_, err := w1.Append(p)
		require.NoError(t, err)
	}
	require.NoError(t, w1.Close())

	// Second session: recovery must hand back every record in order.
	w2, recovered2, err := Open(dir, WithMaxSegmentBytes(128))
	require.NoError(t, err)
	require.Len(t, recovered2, len(payloads))
	for i, r := range recovered2 {
		require.Equal(t, payloads[i], r.Payload, "payload %d mismatch", i)
		require.False(t, r.Offset.IsZero())
	}
	// Ack everything and close cleanly.
	for _, r := range recovered2 {
		w2.Ack(r.Offset)
	}
	require.NoError(t, w2.Close())

	// Third session: dir must be empty again.
	w3, recovered3, err := Open(dir)
	require.NoError(t, err)
	require.Empty(t, recovered3)
	require.NoError(t, w3.Close())
}

func TestWAL_Recovery_TruncatesTornWrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	w, _, err := Open(dir, WithMaxSegmentBytes(1<<20))
	require.NoError(t, err)
	for _, p := range [][]byte{[]byte("first"), []byte("second"), []byte("third")} {
		_, appendErr := w.Append(p)
		require.NoError(t, appendErr)
	}
	require.NoError(t, w.Close())

	// Corrupt the segment by appending half a header — simulates a torn write.
	matches, _ := filepath.Glob(filepath.Join(dir, "*.wal"))
	require.Len(t, matches, 1)
	segPath := matches[0]
	before, err := os.Stat(segPath)
	require.NoError(t, err)

	f, err := os.OpenFile(segPath, os.O_WRONLY|os.O_APPEND, 0)
	require.NoError(t, err)
	_, err = f.Write([]byte{0xFF, 0xFF, 0xFF, 0xFF}) // bogus length prefix, no payload
	require.NoError(t, err)
	require.NoError(t, f.Close())

	// Recovery should truncate the torn trailer and expose exactly the three
	// valid records.
	w2, recovered, err := Open(dir)
	require.NoError(t, err)
	require.Len(t, recovered, 3)
	require.Equal(t, []byte("first"), recovered[0].Payload)
	require.Equal(t, []byte("second"), recovered[1].Payload)
	require.Equal(t, []byte("third"), recovered[2].Payload)
	require.NoError(t, w2.Close())

	after, err := os.Stat(segPath)
	require.NoError(t, err)
	require.Equal(t, before.Size(), after.Size(),
		"torn trailer must be truncated back to the last valid record")
}

func TestWAL_Recovery_DropsCorruptPayload(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	w, _, err := Open(dir)
	require.NoError(t, err)
	_, err = w.Append([]byte("good1"))
	require.NoError(t, err)
	_, err = w.Append([]byte("good2"))
	require.NoError(t, err)
	require.NoError(t, w.Close())

	// Flip the last payload byte so its CRC fails.
	matches, _ := filepath.Glob(filepath.Join(dir, "*.wal"))
	require.Len(t, matches, 1)
	data, err := os.ReadFile(matches[0])
	require.NoError(t, err)
	data[len(data)-1] ^= 0xFF
	require.NoError(t, os.WriteFile(matches[0], data, 0o644))

	// Recovery stops at the first corrupt record.
	w2, recovered, err := Open(dir)
	require.NoError(t, err)
	require.Len(t, recovered, 1, "expected only the first uncorrupted record")
	require.Equal(t, []byte("good1"), recovered[0].Payload)
	require.NoError(t, w2.Close())
}

func TestWAL_MaxBytes_ReturnsErrFull(t *testing.T) {
	t.Parallel()
	// Cap smaller than a single record: the first append lands
	// unconditionally (the soft cap is checked *before* the write, and the
	// live counter is still zero), the second append then sees the cap is
	// already exceeded and bails out with ErrFull.
	w, _ := newTestWAL(t, WithMaxBytes(5))

	_, err := w.Append([]byte("1234567890"))
	require.NoError(t, err)

	_, err = w.Append([]byte("overflow"))
	require.ErrorIs(t, err, ErrFull)
}

func TestWAL_Append_AfterClose_ReturnsErrClosed(t *testing.T) {
	t.Parallel()
	w, _ := newTestWAL(t)
	_, err := w.Append([]byte("pre"))
	require.NoError(t, err)
	require.NoError(t, w.Close())

	_, err = w.Append([]byte("post"))
	require.ErrorIs(t, err, ErrClosed)
}

func TestWAL_Close_IsIdempotent(t *testing.T) {
	t.Parallel()
	w, _ := newTestWAL(t)
	require.NoError(t, w.Close())
	require.NoError(t, w.Close(), "second Close must be a no-op and return nil")
}

func TestWAL_ConcurrentAppendAck(t *testing.T) {
	t.Parallel()
	w, _ := newTestWAL(t)

	const (
		workers          = 8
		appendsPerWorker = 500
	)

	var (
		wg   sync.WaitGroup
		seen atomic.Int64
	)
	for i := range workers {
		wg.Go(func() {
			payload := fmt.Appendf(nil, "worker-%d", i)
			for range appendsPerWorker {
				off, err := w.Append(payload)
				require.NoError(t, err)
				w.Ack(off)
				seen.Add(1)
			}
		})
	}
	wg.Wait()

	require.Equal(t, int64(workers*appendsPerWorker), seen.Load())
	require.Equal(t, int64(0), w.activeUnacked(),
		"all records must be acked after every worker completes")
}

// TestWAL_AppendRacesClose is a regression test for a nil-file panic: an
// Appender that passed the unlocked w.closed.Load() check but then lost the
// mutex race to Close would, on reacquiring the lock, proceed to write
// through w.active.file — which Close had set to nil while w.active itself
// was kept around (unacked > 0). The fix re-checks closed/active.file
// inside Append's critical section and returns ErrClosed instead.
func TestWAL_AppendRacesClose(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	w, _, err := Open(dir,
		WithMaxSegmentBytes(1<<20),
		// Long interval so fsyncLoop doesn't race with Close for the lock
		// and dominate the test schedule; the race we're hunting is
		// between Append and Close, not the fsync goroutine.
		WithFsyncInterval(time.Hour),
	)
	require.NoError(t, err)

	// Seed one unacked record so Close keeps w.active non-nil with a
	// nil file handle — that's the precondition for the old panic.
	_, err = w.Append([]byte("seed"))
	require.NoError(t, err)

	// Appender goroutines hammer the WAL until Close completes.
	const appenders = 8
	var (
		wg            sync.WaitGroup
		stop          atomic.Bool
		panicsSeen    atomic.Int64
		nonClosedErrs atomic.Int64
	)
	for range appenders {
		wg.Go(func() {
			defer func() {
				if r := recover(); r != nil {
					panicsSeen.Add(1)
					t.Errorf("Append panicked: %v", r)
				}
			}()
			payload := []byte("racer")
			for !stop.Load() {
				_, appendErr := w.Append(payload)
				if appendErr != nil && !errors.Is(appendErr, ErrClosed) {
					nonClosedErrs.Add(1)
					t.Errorf("Append: unexpected error %v", appendErr)
					return
				}
			}
		})
	}

	// Give appenders a moment to get in flight, then race Close against
	// them. Close must complete without panicking the appenders.
	time.Sleep(10 * time.Millisecond)
	require.NoError(t, w.Close())
	stop.Store(true)
	wg.Wait()

	require.Zero(t, panicsSeen.Load(), "observed panics from Append")
	require.Zero(t, nonClosedErrs.Load(), "observed non-ErrClosed errors from Append")
}
