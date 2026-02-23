// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package compression_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/cache/compression"
)

func TestGzipCompressor_CompressDecompress_Levels(t *testing.T) {
	levels := []struct {
		name  string
		level int
	}{
		{"fastest", 1},
		{"balanced", 6},
		{"best", 9},
	}

	data := []byte(strings.Repeat("Hello, World! ", 200))

	for _, tt := range levels {
		t.Run(tt.name, func(t *testing.T) {
			c := compression.NewCompressor(64, 0, tt.level)
			ctx := t.Context()

			compressed, err := c.Compress(ctx, data)
			if err != nil {
				t.Fatalf("Compress() error = %v", err)
			}

			decompressed, err := c.Decompress(ctx, compressed)
			if err != nil {
				t.Fatalf("Decompress() error = %v", err)
			}

			if !bytes.Equal(decompressed, data) {
				t.Error("roundtrip data mismatch")
			}
		})
	}
}

func TestGzipCompressor_BelowMinSize(t *testing.T) {
	c := compression.NewCompressor(1024, 0, 6)
	ctx := t.Context()

	small := []byte("tiny")
	compressed, err := c.Compress(ctx, small)
	if err != nil {
		t.Fatalf("Compress() error = %v", err)
	}

	// Should store uncompressed
	decompressed, err := c.Decompress(ctx, compressed)
	if err != nil {
		t.Fatalf("Decompress() error = %v", err)
	}
	if !bytes.Equal(decompressed, small) {
		t.Error("small data roundtrip failed")
	}
}

func TestGzipCompressor_AboveMaxSize(t *testing.T) {
	c := compression.NewCompressor(64, 500, 6)

	large := []byte(strings.Repeat("x", 1000))
	if c.ShouldCompress(large) {
		t.Error("ShouldCompress should return false for data above maxSize")
	}
}

func TestGzipCompressor_EmptyData(t *testing.T) {
	c := compression.NewCompressor(0, 0, 6)
	ctx := t.Context()

	compressed, err := c.Compress(ctx, []byte{})
	if err != nil {
		t.Fatalf("Compress(empty) error = %v", err)
	}

	decompressed, err := c.Decompress(ctx, compressed)
	if err != nil {
		t.Fatalf("Decompress(empty) error = %v", err)
	}
	if len(decompressed) != 0 {
		t.Errorf("expected empty, got %d bytes", len(decompressed))
	}
}

func TestGzipCompressor_ExactlyAtMinSize(t *testing.T) {
	c := compression.NewCompressor(100, 0, 6)

	exactly := make([]byte, 100)
	if !c.ShouldCompress(exactly) {
		t.Error("ShouldCompress should return true at exactly minSize")
	}

	belowMin := make([]byte, 99)
	if c.ShouldCompress(belowMin) {
		t.Error("ShouldCompress should return false below minSize")
	}
}

func TestNewCompressor_InvalidParams(t *testing.T) {
	// Invalid minSize defaults
	c := compression.NewCompressor(-1, 0, 6)
	if c == nil {
		t.Fatal("NewCompressor returned nil with invalid minSize")
	}

	// Invalid level defaults
	c2 := compression.NewCompressor(100, 0, 0)
	if c2 == nil {
		t.Fatal("NewCompressor returned nil with invalid level")
	}

	c3 := compression.NewCompressor(100, 0, 10)
	if c3 == nil {
		t.Fatal("NewCompressor returned nil with level > 9")
	}
}

func TestGzipCompressor_ContextCanceled(t *testing.T) {
	c := compression.NewCompressor(64, 0, 1)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	// Large data triggers context check
	data := []byte(strings.Repeat("x", 2*1024*1024))
	_, err := c.Compress(ctx, data)
	if err == nil {
		t.Error("Compress with canceled context should return error")
	}
}

func TestGzipCompressor_DecompressInvalidData(t *testing.T) {
	c := compression.NewCompressor(64, 0, 6)
	ctx := t.Context()

	// Random data — should fall back to returning raw data
	raw := []byte("not compressed data")
	result, err := c.Decompress(ctx, raw)
	if err != nil {
		t.Fatalf("Decompress(invalid) error = %v", err)
	}
	if !bytes.Equal(result, raw) {
		t.Error("invalid data should be returned as-is")
	}
}

func TestGzipCompressor_LargePayload_Roundtrip(t *testing.T) {
	c := compression.NewCompressor(64, 0, 1)
	ctx := t.Context()

	// 1.5MB of compressible data
	data := []byte(strings.Repeat("ABCDEFGH", 200*1024))

	compressed, err := c.Compress(ctx, data)
	if err != nil {
		t.Fatalf("Compress(large) error = %v", err)
	}

	decompressed, err := c.Decompress(ctx, compressed)
	if err != nil {
		t.Fatalf("Decompress(large) error = %v", err)
	}

	if !bytes.Equal(decompressed, data) {
		t.Errorf("large payload roundtrip failed: got %d bytes, want %d", len(decompressed), len(data))
	}
}

func TestPresetValues(t *testing.T) {
	if compression.PresetNone != 0 {
		t.Errorf("PresetNone = %d", compression.PresetNone)
	}
	if compression.PresetFast != 1 {
		t.Errorf("PresetFast = %d", compression.PresetFast)
	}
	if compression.PresetBalanced != 2 {
		t.Errorf("PresetBalanced = %d", compression.PresetBalanced)
	}
	if compression.PresetBest != 3 {
		t.Errorf("PresetBest = %d", compression.PresetBest)
	}
}

func TestMetadata_Fields(t *testing.T) {
	m := compression.Metadata{
		IsCompressed: true,
		OriginalSize: 1024,
	}
	if !m.IsCompressed {
		t.Error("IsCompressed should be true")
	}
	if m.OriginalSize != 1024 {
		t.Errorf("OriginalSize = %d, want 1024", m.OriginalSize)
	}
}

func TestDefaultConstants(t *testing.T) {
	if compression.DefaultMinSize != 1024 {
		t.Errorf("DefaultMinSize = %d", compression.DefaultMinSize)
	}
	if compression.DefaultMaxSize != 0 {
		t.Errorf("DefaultMaxSize = %d", compression.DefaultMaxSize)
	}
	if compression.DefaultLevel != 6 {
		t.Errorf("DefaultLevel = %d", compression.DefaultLevel)
	}
}
