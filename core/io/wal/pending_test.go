// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package wal_test

import (
	"github.com/altessa-s/go-atlas/core/io/wal"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestReadPendingLimitSkipAndDuplicateAck(t *testing.T) {
	t.Parallel()
	w, _, err := wal.Open(t.TempDir(), wal.WithMaxSegmentBytes(20))
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
