// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package wal_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/io/wal"
)

func TestReadPendingLimitSkipAndDuplicateAck(t *testing.T) {
	t.Parallel()
	w, _, err := wal.Open(filepath.Join(t.TempDir(), "wal"), wal.WithMaxSegmentBytes(20))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, w.Close()) })
	var offsets []wal.Offset
	for _, value := range []string{"a", "b", "c", "d"} {
		off, err := w.Append([]byte(value))
		require.NoError(t, err)
		offsets = append(offsets, off)
	}
	w.Ack(offsets[1])
	w.Ack(offsets[1])
	require.Equal(t, int64(3), w.Stats().Pending)
	records, err := w.ReadPending(1, func(off wal.Offset) bool { return off == offsets[0] })
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, offsets[2], records[0].Offset)
	w.Ack(offsets[0])
	records, err = w.ReadPending(10, nil)
	require.NoError(t, err)
	require.Len(t, records, 2)
	require.Equal(t, []byte("c"), records[0].Payload)
	require.Equal(t, []byte("d"), records[1].Payload)
	for _, record := range records {
		w.Ack(record.Offset)
	}
	require.Zero(t, w.Stats().Pending)
	off, err := w.Append([]byte("e"))
	require.NoError(t, err)
	records, err = w.ReadPending(1, nil)
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, off, records[0].Offset)
	w.Ack(off)
}
func TestPendingReadAfterReopen(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o700))
	w, _, err := wal.Open(dir)
	require.NoError(t, err)
	_, err = w.Append([]byte("pending"))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	w, recovered, err := wal.Open(dir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, w.Close()) })
	got, err := w.ReadPending(1, nil)
	require.NoError(t, err)
	require.Equal(t, recovered, got)
	w.Ack(got[0].Offset)
	require.NoError(t, w.Close())
	_, err = w.ReadPending(1, nil)
	require.ErrorIs(t, err, wal.ErrClosed)
}

func TestReadPendingSkippedRecordsRemainRetryable(t *testing.T) {
	t.Parallel()
	w, _, err := wal.Open(filepath.Join(t.TempDir(), "wal"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, w.Close()) })
	var offsets []wal.Offset
	for i := range 6 {
		off, err := w.Append([]byte{byte('a' + i)})
		require.NoError(t, err)
		offsets = append(offsets, off)
	}
	inFlight := map[wal.Offset]struct{}{offsets[0]: {}, offsets[1]: {}, offsets[3]: {}}
	skip := func(off wal.Offset) bool { _, ok := inFlight[off]; return ok }
	w.Ack(offsets[4])
	records, err := w.ReadPending(10, skip)
	require.NoError(t, err)
	require.Equal(t, []wal.Offset{offsets[2], offsets[5]}, recordOffsets(records))
	// Delivery of the in-flight batch failed: nothing was acknowledged, so a
	// retry without the skip set must return every unacknowledged record.
	records, err = w.ReadPending(10, nil)
	require.NoError(t, err)
	require.Equal(t, []wal.Offset{offsets[0], offsets[1], offsets[2], offsets[3], offsets[5]}, recordOffsets(records))
	w.Ack(offsets[0])
	w.Ack(offsets[2])
	records, err = w.ReadPending(10, nil)
	require.NoError(t, err)
	require.Equal(t, []wal.Offset{offsets[1], offsets[3], offsets[5]}, recordOffsets(records))
	require.Equal(t, []byte("b"), records[0].Payload)
}

func TestReadPendingAcrossReadWindows(t *testing.T) {
	t.Parallel()
	w, _, err := wal.Open(filepath.Join(t.TempDir(), "wal"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, w.Close()) })
	// Sizes straddle and exceed the 64 KiB internal read window.
	sizes := []int{100, 40 << 10, 30 << 10, 200 << 10, 1, 64<<10 - 8, 5, 70 << 10}
	var want []wal.Record
	for i, size := range sizes {
		payload := bytes.Repeat([]byte{byte(i + 1)}, size)
		off, err := w.Append(payload)
		require.NoError(t, err)
		want = append(want, wal.Record{Offset: off, Payload: payload})
	}
	got, err := w.ReadPending(len(sizes), nil)
	require.NoError(t, err)
	require.Equal(t, want, got)
	got, err = w.ReadPending(2, func(off wal.Offset) bool { return off != want[3].Offset && off != want[7].Offset })
	require.NoError(t, err)
	require.Equal(t, []wal.Record{want[3], want[7]}, got)
}

func TestReadPendingDetectsCorruptionAndTruncation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o700))
	w, _, err := wal.Open(dir, wal.WithMaxSegmentBytes(32))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, w.Close()) })
	_, err = w.Append([]byte("first-record"))
	require.NoError(t, err)
	_, err = w.Append([]byte("second-record"))
	require.NoError(t, err)
	segments, err := filepath.Glob(filepath.Join(dir, "*.wal"))
	require.NoError(t, err)
	require.Len(t, segments, 2)
	f, err := os.OpenFile(segments[0], os.O_WRONLY, 0)
	require.NoError(t, err)
	_, err = f.WriteAt([]byte{'X'}, 8)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	_, err = w.ReadPending(1, nil)
	require.ErrorIs(t, err, wal.ErrCorruptRecord)
	require.NoError(t, os.Truncate(segments[0], 4))
	_, err = w.ReadPending(1, nil)
	require.ErrorIs(t, err, io.EOF)
}

func recordOffsets(records []wal.Record) []wal.Offset {
	offsets := make([]wal.Offset, 0, len(records))
	for _, record := range records {
		offsets = append(offsets, record.Offset)
	}
	return offsets
}

// TestReadPending_ConcurrentWithGroupCommit runs producers, a reader that
// acks what it reads, and segment rolls together: every appended record must
// be delivered exactly once, and nothing may stay pending.
func TestReadPending_ConcurrentWithGroupCommit(t *testing.T) {
	t.Parallel()
	w, _, err := wal.Open(filepath.Join(t.TempDir(), "wal"), wal.WithMaxSegmentBytes(2<<10), wal.WithFsyncInterval(time.Hour))
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close() })

	const producers, perProducer = 6, 300
	const total = producers * perProducer
	var wg sync.WaitGroup
	for p := range producers {
		wg.Go(func() {
			for i := range perProducer {
				if _, err := w.Append(fmt.Appendf(nil, "%d-%d", p, i)); err != nil {
					t.Errorf("Append: %v", err)
					return
				}
			}
		})
	}

	seen := make(map[string]int, total)
	deadline := time.Now().Add(30 * time.Second)
	for len(seen) < total && time.Now().Before(deadline) {
		records, err := w.ReadPending(64, nil)
		require.NoError(t, err)
		for _, r := range records {
			seen[string(r.Payload)]++
			w.Ack(r.Offset)
		}
	}
	wg.Wait()

	require.Len(t, seen, total)
	for payload, n := range seen {
		require.Equal(t, 1, n, "record %q delivered %d times", payload, n)
	}
	rest, err := w.ReadPending(64, nil)
	require.NoError(t, err)
	require.Empty(t, rest)
	require.Zero(t, w.Stats().Pending)
}
