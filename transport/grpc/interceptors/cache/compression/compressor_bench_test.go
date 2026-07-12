// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package compression

import (
	"bytes"
	"testing"
)

// benchRoundTripPayloadSize is a representative cached-response payload size (~2 KB).
const benchRoundTripPayloadSize = 2048

// benchRoundTripPayload builds a compressible but non-trivial payload of the
// given size from repeated JSON-like records, approximating serialized
// response data.
func benchRoundTripPayload(size int) []byte {
	pattern := []byte(`{"id":12345,"name":"benchmark-record","active":true,"score":98.7},`)
	buf := bytes.Repeat(pattern, size/len(pattern)+1)
	return buf[:size]
}

func BenchmarkGzipCompressor_RoundTrip(b *testing.B) {
	c := NewCompressor(DefaultMinSize, DefaultMaxSize, DefaultLevel)
	data := benchRoundTripPayload(benchRoundTripPayloadSize)
	ctx := b.Context()

	b.ReportAllocs()
	for b.Loop() {
		compressed, err := c.Compress(ctx, data)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := c.Decompress(ctx, compressed); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGzipCompressor_Compress(b *testing.B) {
	c := NewCompressor(10, 0, 6)
	ctx := b.Context()
	data := make([]byte, 4096)
	for i := range data {
		data[i] = byte(i%26) + 'a'
	}
	b.ReportAllocs()
	for b.Loop() {
		c.Compress(ctx, data) //nolint:errcheck
	}
}

func BenchmarkGzipCompressor_Decompress(b *testing.B) {
	c := NewCompressor(10, 0, 6)
	ctx := b.Context()
	data := make([]byte, 4096)
	for i := range data {
		data[i] = byte(i%26) + 'a'
	}
	compressed, _ := c.Compress(ctx, data)
	b.ReportAllocs()
	for b.Loop() {
		c.Decompress(ctx, compressed) //nolint:errcheck
	}
}

func BenchmarkNoOpCompressor_Compress(b *testing.B) {
	c := NewNoOpCompressor()
	ctx := b.Context()
	data := make([]byte, 1024)
	b.ReportAllocs()
	for b.Loop() {
		c.Compress(ctx, data) //nolint:errcheck
	}
}
