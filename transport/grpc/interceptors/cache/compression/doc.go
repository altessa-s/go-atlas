// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package compression provides high-performance gzip compression for cache operations.
//
// Use [NewCompressor] to create a [GzipCompressor] (the only [Compressor]
// implementation). Call [GzipCompressor.ShouldCompress] to check size
// thresholds before compressing, then [GzipCompressor.Compress] /
// [GzipCompressor.Decompress] for the actual operations.
//
// The package implements optimized gzip compression with the following features:
//   - 4-tier buffer pooling for memory efficiency (8KB, 64KB, 256KB, 2MB+)
//   - Zero-copy operations for small payloads
//   - Streaming compression for large payloads with context cancellation support
//   - Multi-layered security protection against zip bomb attacks
//   - Configurable compression presets ([PresetNone], [PresetFast],
//     [PresetBalanced], [PresetBest])
//
// Payloads exceeding maxPayloadSize cause [ErrPayloadTooLarge].
// [GzipCompressor] is safe for concurrent use; all internal buffer pools
// are managed with [sync.Pool].
//
// Example:
//
//	// Create a balanced compressor for general use
//	compressor := compression.NewCompressor(1024, 0, 6)
//
//	// Compress data
//	compressed, err := compressor.Compress(ctx, data)
//
//	// Decompress data
//	original, err := compressor.Decompress(ctx, compressed)
package compression
