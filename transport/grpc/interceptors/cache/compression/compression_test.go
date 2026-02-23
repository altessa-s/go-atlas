// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package compression

import (
	"testing"
)

func TestNoOpCompressor(t *testing.T) {
	c := NewNoOpCompressor()

	data := []byte("hello world")
	compressed, err := c.Compress(t.Context(), data)
	if err != nil {
		t.Fatal(err)
	}
	if string(compressed) != string(data) {
		t.Fatal("NoOp should return data unchanged")
	}

	decompressed, err := c.Decompress(t.Context(), data)
	if err != nil {
		t.Fatal(err)
	}
	if string(decompressed) != string(data) {
		t.Fatal("NoOp should return data unchanged")
	}

	if c.ShouldCompress(data) {
		t.Fatal("NoOp should never compress")
	}
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
			if got := c.ShouldCompress(data); got != tt.want {
				t.Fatalf("ShouldCompress(%d) = %v, want %v", tt.size, got, tt.want)
			}
		})
	}
}

func TestGzipCompressor_ShouldCompress_MaxSize(t *testing.T) {
	c := NewCompressor(100, 500, 6)
	if c.ShouldCompress(make([]byte, 600)) {
		t.Fatal("should not compress above maxSize")
	}
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
	if err != nil {
		t.Fatal(err)
	}

	decompressed, err := c.Decompress(ctx, compressed)
	if err != nil {
		t.Fatal(err)
	}
	if string(decompressed) != string(data) {
		t.Fatal("roundtrip failed")
	}
}

func TestGzipCompressor_SmallData(t *testing.T) {
	c := NewCompressor(1024, 0, 6)
	ctx := t.Context()

	data := []byte("small")
	compressed, err := c.Compress(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	// Small data should be stored uncompressed
	decompressed, err := c.Decompress(ctx, compressed)
	if err != nil {
		t.Fatal(err)
	}
	if string(decompressed) != string(data) {
		t.Fatal("roundtrip failed for small data")
	}
}

func TestNewCompressor_Defaults(t *testing.T) {
	c := NewCompressor(0, 0, 0)
	if c.minSize != DefaultMinSize {
		t.Fatalf("minSize = %d, want %d", c.minSize, DefaultMinSize)
	}
	if c.compressionLevel != DefaultLevel {
		t.Fatalf("level = %d, want %d", c.compressionLevel, DefaultLevel)
	}
}

func TestPresetConstants(t *testing.T) {
	if PresetNone != 0 {
		t.Fatal("PresetNone should be 0")
	}
	if PresetFast != 1 {
		t.Fatal("PresetFast should be 1")
	}
}

func TestErrPayloadTooLarge(t *testing.T) {
	if ErrPayloadTooLarge == nil {
		t.Fatal("should not be nil")
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
