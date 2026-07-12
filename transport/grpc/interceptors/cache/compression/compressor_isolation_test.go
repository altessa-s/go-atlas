// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package compression

import (
	"bytes"
	"compress/gzip"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// makePattern builds deterministic, seed-dependent content so two payloads with
// different seeds are guaranteed to differ byte-for-byte.
func makePattern(seed byte, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = seed ^ byte(i*31+int(seed))
	}
	return b
}

// gzipBytes returns the gzip-compressed form of data.
func gzipBytes(tb testing.TB, data []byte) []byte {
	tb.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, err := w.Write(data)
	require.NoError(tb, err)
	require.NoError(tb, w.Close())
	return buf.Bytes()
}

// newGzipReader returns a *gzip.Reader over the given compressed bytes.
func newGzipReader(tb testing.TB, gz []byte) *gzip.Reader {
	tb.Helper()
	r, err := gzip.NewReader(bytes.NewReader(gz))
	require.NoError(tb, err)
	return r
}

// TestStreamDecompress_ResultOwnedNotAliased pins the fix for the cross-request
// buffer-aliasing leak: streamDecompress must return a caller-owned slice, not a
// slice backed by a pooled array. Previously the accumulator (and its growth
// buffers) were drawn from a sync.Pool and returned to the pool via defer while
// the returned slice still aliased them; a subsequent decompression could pull
// the same backing array and overwrite a prior result — one request's payload
// leaking into another. Here we hold the first result and prove later
// decompressions never mutate it.
func TestStreamDecompress_ResultOwnedNotAliased(t *testing.T) {
	t.Parallel()

	c := NewCompressor(0, 0, gzip.DefaultCompression)
	ctx := t.Context()

	const size = 300 * 1024 // 300 KiB routes through the growth path
	payloadA := makePattern('A', size)
	payloadB := makePattern('B', size)
	gzA := gzipBytes(t, payloadA)
	gzB := gzipBytes(t, payloadB)

	// Undersized hint forces the accumulator to grow at least once, exercising
	// growBuffer/growBufferDirect while staying within the 2x size guard.
	expectedSize := size * 3 / 4

	rA, err := c.streamDecompress(ctx, newGzipReader(t, gzA), expectedSize)
	require.NoError(t, err)
	require.Equal(t, payloadA, rA)

	// Repeatedly decompress a different payload. If rA aliased a pooled buffer,
	// one of these calls would recycle it and corrupt rA.
	for range 50 {
		rB, err := c.streamDecompress(ctx, newGzipReader(t, gzB), expectedSize)
		require.NoError(t, err)
		require.Equal(t, payloadB, rB)
	}

	require.Equal(t, payloadA, rA, "first decompression result was mutated by later calls (buffer aliasing)")
}

// TestStreamDecompress_ConcurrentIsolation stresses the same guarantee under
// concurrency. Run with -race: with pooled result buffers, concurrent callers
// would read each other's decompressed data through a shared backing array.
func TestStreamDecompress_ConcurrentIsolation(t *testing.T) {
	t.Parallel()

	c := NewCompressor(0, 0, gzip.DefaultCompression)
	ctx := t.Context()

	const (
		size       = 200 * 1024
		goroutines = 16
		iterations = 20
	)
	expectedSize := size * 3 / 4

	// Each goroutine owns a distinct payload and asserts every round-trip yields
	// exactly its own bytes back.
	var wg sync.WaitGroup
	for g := range goroutines {
		payload := makePattern(byte(g), size)
		gz := gzipBytes(t, payload)
		wg.Go(func() {
			for range iterations {
				got, err := c.streamDecompress(ctx, newGzipReader(t, gz), expectedSize)
				if err != nil {
					t.Errorf("goroutine %d: streamDecompress: %v", g, err)
					return
				}
				if !bytes.Equal(got, payload) {
					t.Errorf("goroutine %d: decompressed data does not match own payload (cross-request contamination)", g)
					return
				}
			}
		})
	}
	wg.Wait()
}
