// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package wal_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/io/wal"
)

func BenchmarkReadPending(b *testing.B) {
	w, _, err := wal.Open(b.TempDir())
	require.NoError(b, err)
	b.Cleanup(func() { require.NoError(b, w.Close()) })
	for range 100 {
		_, err = w.Append(make([]byte, 64))
		require.NoError(b, err)
	}
	b.ReportAllocs()
	for b.Loop() {
		records, err := w.ReadPending(100, nil)
		require.NoError(b, err)
		require.Len(b, records, 100)
	}
}

// openInFlightWAL appends total records and returns a skip function that
// excludes the first inFlight of them, mimicking the dispatch claim path.
func openInFlightWAL(b *testing.B, total, inFlight int) (*wal.WAL, func(wal.Offset) bool) {
	b.Helper()
	w, _, err := wal.Open(b.TempDir(), wal.WithFsyncInterval(time.Hour))
	require.NoError(b, err)
	b.Cleanup(func() { require.NoError(b, w.Close()) })
	skipped := make(map[wal.Offset]struct{}, inFlight)
	for i := range total {
		off, err := w.Append(make([]byte, 256))
		require.NoError(b, err)
		if i < inFlight {
			skipped[off] = struct{}{}
		}
	}
	return w, func(off wal.Offset) bool {
		_, ok := skipped[off]
		return ok
	}
}

func BenchmarkReadPendingSkipInFlight(b *testing.B) {
	w, skip := openInFlightWAL(b, 1000, 900)
	b.ReportAllocs()
	for b.Loop() {
		records, err := w.ReadPending(100, skip)
		require.NoError(b, err)
		require.Len(b, records, 100)
	}
}

// BenchmarkAppendDuringReadPending measures Append throughput while another
// goroutine continuously claims batches, the dispatch durable-path contention.
func BenchmarkAppendDuringReadPending(b *testing.B) {
	w, skip := openInFlightWAL(b, 1000, 900)
	stop := make(chan struct{})
	var reads atomic.Int64
	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			if _, err := w.ReadPending(100, skip); err != nil {
				b.Error(err)
				return
			}
			reads.Add(1)
		}
	})
	payload := make([]byte, 256)
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := w.Append(payload); err != nil {
				b.Error(err)
				return
			}
		}
	})
	b.StopTimer()
	close(stop)
	wg.Wait()
	b.ReportMetric(float64(reads.Load())/float64(b.N), "reads/op")
}
