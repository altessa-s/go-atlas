// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ocsp

import (
	"bytes"
	"testing"
	"time"
)

func BenchmarkCompressData(b *testing.B) {
	data := bytes.Repeat([]byte("test data for compression "), 100)
	for b.Loop() {
		_, _ = compressData(data)
	}
}

func BenchmarkDecompressData(b *testing.B) {
	data := bytes.Repeat([]byte("test data for compression "), 100)
	compressed, err := compressData(data)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		_, _ = decompressData(compressed)
	}
}

func BenchmarkCompressDecompressRoundtrip(b *testing.B) {
	data := bytes.Repeat([]byte("OCSP response data simulation "), 50)
	for b.Loop() {
		compressed, _ := compressData(data)
		_, _ = decompressData(compressed)
	}
}

func BenchmarkNewOCSPStapler(b *testing.B) {
	for b.Loop() {
		_ = NewOCSPStapler()
	}
}

func BenchmarkPrepareCacheEntry_NoCompression(b *testing.B) {
	s := NewOCSPStapler()
	data := bytes.Repeat([]byte("test"), 100)
	ctx := b.Context()
	nextUpdate := time.Now().Add(24 * time.Hour)
	b.ResetTimer()
	for b.Loop() {
		_ = s.prepareCacheEntry(ctx, data, nextUpdate)
	}
}

func BenchmarkPrepareCacheEntry_WithCompression(b *testing.B) {
	s := NewOCSPStapler(WithCompression())
	data := bytes.Repeat([]byte("test"), 100)
	ctx := b.Context()
	nextUpdate := time.Now().Add(24 * time.Hour)
	b.ResetTimer()
	for b.Loop() {
		_ = s.prepareCacheEntry(ctx, data, nextUpdate)
	}
}
