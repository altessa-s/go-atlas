// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package compression

import (
	"bytes"
	"compress/gzip"
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNoOpCompressor(t *testing.T) {
	c := NewNoOpCompressor()

	data := []byte("hello world")
	compressed, err := c.Compress(t.Context(), data)
	require.NoError(t, err)
	require.Equal(t, string(data), string(compressed))

	decompressed, err := c.Decompress(t.Context(), data)
	require.NoError(t, err)
	require.Equal(t, string(data), string(decompressed))

	require.False(t, c.ShouldCompress(data), "NoOp should never compress")
}

func TestGzipCompressor_ShouldCompress(t *testing.T) {
	c := NewCompressor(1024, 0, 6)

	tests := []struct {
		name string
		size int
		want bool
	}{
		{"below_min", 512, false},
		{"at_min", 1024, true},
		{"above_min", 2048, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make([]byte, tt.size)
			got := c.ShouldCompress(data)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestGzipCompressor_ShouldCompress_MaxSize(t *testing.T) {
	c := NewCompressor(100, 500, 6)
	require.False(t, c.ShouldCompress(make([]byte, 600)), "should not compress above maxSize")
}

func TestGzipCompressor_CompressDecompress(t *testing.T) {
	c := NewCompressor(10, 0, 6)
	ctx := t.Context()

	// Data large enough to benefit from compression
	data := make([]byte, 2048)
	for i := range data {
		data[i] = byte(i%26) + 'a'
	}

	compressed, err := c.Compress(ctx, data)
	require.NoError(t, err)

	decompressed, err := c.Decompress(ctx, compressed)
	require.NoError(t, err)
	require.Equal(t, string(data), string(decompressed))
}

func TestGzipCompressor_SmallData(t *testing.T) {
	c := NewCompressor(1024, 0, 6)
	ctx := t.Context()

	data := []byte("small")
	compressed, err := c.Compress(ctx, data)
	require.NoError(t, err)
	// Small data should be stored uncompressed
	decompressed, err := c.Decompress(ctx, compressed)
	require.NoError(t, err)
	require.Equal(t, string(data), string(decompressed))
}

func TestNewCompressor_Defaults(t *testing.T) {
	c := NewCompressor(0, 0, 0)
	require.Equal(t, DefaultMinSize, c.minSize)
	require.Equal(t, DefaultLevel, c.compressionLevel)
}

// TestGzipCompressor_Decompress_ActualOutputExceedsDeclaredSize verifies that a
// crafted blob whose declared OriginalSize passes the compression-ratio gates
// but whose gzip stream actually expands far beyond it is rejected instead of
// being decompressed unbounded (zip-bomb protection on the small-payload path).
func TestGzipCompressor_Decompress_ActualOutputExceedsDeclaredSize(t *testing.T) {
	gzipBytes := func(t *testing.T, payload []byte) []byte {
		t.Helper()
		var buf bytes.Buffer
		w := gzip.NewWriter(&buf)
		_, err := w.Write(payload)
		require.NoError(t, err)
		require.NoError(t, w.Close())
		return buf.Bytes()
	}

	tests := []struct {
		name         string
		payload      func() []byte
		declaredSize int
	}{
		{
			// Zeros compress to a few KB, keeping the blob below
			// maxPooledBufferSize: exercises the pooled io.ReadAll path.
			name:         "small_compressed_blob",
			payload:      func() []byte { return make([]byte, 4*1024*1024) },
			declaredSize: 40 * 1024,
		},
		{
			// Incompressible data keeps the compressed blob above
			// maxPooledBufferSize: exercises the streaming path.
			name: "large_compressed_blob",
			payload: func() []byte {
				r := rand.New(rand.NewPCG(1, 2))
				p := make([]byte, 3*1024*1024)
				for i := range p {
					p[i] = byte(r.UintN(256))
				}
				return p
			},
			declaredSize: 512 * 1024,
		},
	}

	c := NewCompressor(10, 0, 6)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blob, err := encodeCompressedData(compressedData{
				Metadata: Metadata{IsCompressed: true, OriginalSize: tt.declaredSize},
				Data:     gzipBytes(t, tt.payload()),
			})
			require.NoError(t, err)

			_, err = c.Decompress(t.Context(), blob)
			require.ErrorIs(t, err, ErrBufferSizeExceeded)
		})
	}
}

func BenchmarkGzipCompressor_Compress(b *testing.B) {
	c := NewCompressor(10, 0, 6)
	ctx := b.Context()
	data := make([]byte, 4096)
	for i := range data {
		data[i] = byte(i%26) + 'a'
	}
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
	for b.Loop() {
		c.Decompress(ctx, compressed) //nolint:errcheck
	}
}

func BenchmarkNoOpCompressor_Compress(b *testing.B) {
	c := NewNoOpCompressor()
	ctx := b.Context()
	data := make([]byte, 1024)
	for b.Loop() {
		c.Compress(ctx, data) //nolint:errcheck
	}
}
