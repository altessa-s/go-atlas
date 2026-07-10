// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package compression

import (
	"bytes"
	"context"
	"errors"
	"sync"
)

// Compression mode constants - simplified for gzip-only implementation
const (
	// compressedFlag indicates data is gzip compressed
	compressedFlag uint8 = 1
	// uncompressedFlag indicates data is not compressed
	uncompressedFlag uint8 = 0
)

// Preset defines common compression presets for easy configuration.
type Preset int

const (
	// PresetNone disables compression entirely for all data
	PresetNone Preset = iota
	// PresetFast uses gzip level 1 compression, optimized for CPU-constrained environments
	// Provides fastest compression speed with moderate space savings
	PresetFast
	// PresetBalanced uses gzip level 6 compression, providing optimal balance of speed and compression
	// Recommended for most production environments as the default setting
	PresetBalanced
	// PresetBest uses gzip level 9 compression, optimized for bandwidth-constrained environments
	// Provides maximum compression ratio at the cost of increased CPU usage
	PresetBest
)

// Compressor handles gzip compression and decompression for cache operations.
// This is a simplified interface focused only on gzip compression.
type Compressor interface {
	// Compress compresses the data using gzip and returns the result with headers
	Compress(ctx context.Context, data []byte) ([]byte, error)
	// Decompress decompresses gzip-compressed data and returns the original data
	Decompress(ctx context.Context, data []byte) ([]byte, error)
	// ShouldCompress determines if data should be compressed based on size thresholds
	ShouldCompress(data []byte) bool
}

// GzipCompressor provides optimized gzip compression with size-based decisions.
// This is the only supported [Compressor] implementation. Create instances
// with [NewCompressor].
//
// GzipCompressor is safe for concurrent use; all shared state is managed
// through package-level [sync.Pool] instances.
type GzipCompressor struct {
	minSize          int // Minimum size in bytes to trigger compression
	maxSize          int // Maximum size in bytes to allow compression (0 = no limit)
	compressionLevel int // Gzip compression level (1=fastest, 6=balanced, 9=best compression)
}

// Metadata contains metadata about gzip compressed data.
type Metadata struct {
	IsCompressed bool `json:"compressed"`
	OriginalSize int  `json:"orig_size"`
}

// compressedData represents compressed data with metadata.
type compressedData struct {
	Metadata Metadata `json:"meta"`
	Data     []byte   `json:"data"`
}

const (
	// DefaultMinSize is the minimum size in bytes to trigger compression
	DefaultMinSize = 1024 // 1KB
	// DefaultMaxSize is the maximum size in bytes to allow compression (0 = no limit)
	DefaultMaxSize = 0
	// DefaultLevel is the default compression level
	DefaultLevel = 6
)

const (
	// Binary format constants for optimized encoding
	headerSize  = 9      // Optimized header: magic(2) + version(1) + algorithm(1) + original_size(4) + compressed_size(1)
	magicBytes  = 0x8C8A // Magic bytes for validation (chosen to avoid common patterns)
	versionByte = 1      // Format version for future compatibility
)

// Buffer pool constants optimized for different payload sizes
const (
	// SmallBufferSize for payloads up to 8KB
	SmallBufferSize = 8 * 1024 // 8KB for small payloads
	// MediumBufferSize for payloads 8KB-256KB
	MediumBufferSize = 64 * 1024 // 64KB for medium payloads
	// LargeBufferSize for payloads 256KB+
	LargeBufferSize = 256 * 1024 // 256KB for large payloads
	// defaultBufferSize maintained for backward compatibility
	defaultBufferSize   = MediumBufferSize
	maxPooledBufferSize = 2 * 1024 * 1024 // 2MB max buffer size to return to pool
)

// Zero-copy optimization constants
const (
	// smallDataThreshold for zero-copy vs pooled buffer decision
	smallDataThreshold = 8 * 1024 // 8KB - below this, use zero-copy techniques
)

// Header field offsets for zero-copy operations
const (
	magicOffset        = 0 // Magic bytes start at offset 0
	versionOffset      = 2 // Version byte at offset 2
	reservedOffset     = 3 // Reserved byte at offset 3
	originalSizeOffset = 4 // Original size (4 bytes) at offset 4
	compressedOffset   = 8 // Compression flag at offset 8
)

// Memory usage limits
const (
	maxPayloadSize        = 10 * 1024 * 1024 // 10MB - absolute maximum payload size
	maxCompressionMemory  = 2 * 1024 * 1024  // 2MB - maximum memory during compression
	largePayloadThreshold = 1 * 1024 * 1024  // 1MB - threshold for large payload handling
)

// Security limits for compression
const (
	// maxCompressionRatio is the maximum allowed compression ratio to prevent zip bomb attacks
	maxCompressionRatio = 300 // 300:1 maximum compression ratio
	// maxSuspiciousCompressionRatio is a stricter limit for very small payloads that compress suspiciously well
	maxSuspiciousCompressionRatio = 200 // 200:1 maximum ratio for very small payloads
	// suspiciousPayloadThreshold defines the size below which stricter limits apply
	suspiciousPayloadThreshold = 100 // 100 bytes - below this size, use stricter ratio
	// maxBufferSize is the absolute maximum buffer size to prevent memory exhaustion
	maxBufferSize = 50 * 1024 * 1024 // 50MB absolute maximum buffer size
	// bufferSafetyMargin prevents allocation near the absolute limit
	bufferSafetyMargin = 1024 * 1024 // 1MB safety margin for buffer growth
	// maxDecompressionSize is the absolute maximum size for decompressed data
	maxDecompressionSize = 100 * 1024 * 1024 // 100MB absolute maximum decompressed size
)

// Compression ratio estimates for memory allocation
const (
	// compressionRatioEstimate is the estimated compression ratio (1/3 of original size)
	compressionRatioEstimate = 3
	// decompressionRatioEstimate is the estimated decompression ratio (3x original size)
	decompressionRatioEstimate = 3
	// maxDecompressionSizeMultiplier is the maximum allowed size multiplier for decompressed data
	maxDecompressionSizeMultiplier = 2
	// bufferGrowthMultiplier is the factor by which buffers grow when needed
	bufferGrowthMultiplier = 2
)

// OptimizedHeader represents the binary header structure
type OptimizedHeader struct {
	Magic        uint16 // 2 bytes: Magic number for format identification
	Version      uint8  // 1 byte: Format version
	Reserved     uint8  // 1 byte: Reserved for future use (was algorithm)
	OriginalSize uint32 // 4 bytes: Original data size
	IsCompressed uint8  // 1 byte: Compression flag (0=not compressed, 1=gzip compressed)
}

// Memory tracking errors for payload size and memory limit enforcement.
var (
	// ErrPayloadTooLarge is returned by [GzipCompressor.Compress] and
	// [GzipCompressor.Decompress] when the input data exceeds the maximum
	// allowed payload size (maxPayloadSize) or when decompressed data
	// exceeds the decompression safety limit.
	ErrPayloadTooLarge = errors.New("payload exceeds maximum size limit")
)

// Tiered buffer pools for memory optimization
var (
	// smallBufferPool for small payloads (up to 8KB)
	smallBufferPool = sync.Pool{
		New: func() any {
			return bytes.NewBuffer(make([]byte, 0, SmallBufferSize))
		},
	}

	// mediumBufferPool for medium payloads (8KB-256KB)
	mediumBufferPool = sync.Pool{
		New: func() any {
			return bytes.NewBuffer(make([]byte, 0, MediumBufferSize))
		},
	}

	// largeBufferPool for large payloads (256KB+)
	largeBufferPool = sync.Pool{
		New: func() any {
			return bytes.NewBuffer(make([]byte, 0, LargeBufferSize))
		},
	}

	// Note: bytesBufferPool now delegates to sized pools for backward compatibility
)

// NewCompressor creates a new GzipCompressor with the specified configuration.
// The minSize parameter sets the minimum data size (in bytes) to trigger compression.
// The maxSize parameter sets the maximum data size to allow compression (0 = no limit).
// The level parameter sets the gzip compression level (1=fastest, 6=balanced, 9=best compression).
//
// Invalid parameters are replaced with sensible defaults to ensure reliable operation.
// Returns a configured Compressor ready for concurrent use.
func NewCompressor(minSize, maxSize, level int) *GzipCompressor {
	if minSize <= 0 {
		minSize = DefaultMinSize
	}

	if level <= 0 || level > 9 {
		level = DefaultLevel
	}

	return &GzipCompressor{
		minSize:          minSize,
		maxSize:          maxSize,
		compressionLevel: level,
	}
}

// ShouldCompress determines if data should be compressed based on configured size thresholds.
// Returns true if the data meets the minimum size requirement and doesn't exceed the maximum size limit.
// Small data typically doesn't benefit from compression due to header overhead.
// Large data may be skipped if it exceeds memory limits.
func (c *GzipCompressor) ShouldCompress(data []byte) bool {
	size := len(data)

	// Too small to benefit from compression
	if size < c.minSize {
		return false
	}

	// Too large (if maxSize is set)
	if c.maxSize > 0 && size > c.maxSize {
		return false
	}

	return true
}

// Compress compresses the input data using gzip with automatic size-based optimizations.
// The method automatically selects between standard and streaming compression based on payload size.
// Small payloads that don't benefit from compression are stored uncompressed with appropriate headers.
//
// Large payloads use streaming compression to control memory usage and prevent memory exhaustion.
// The method includes compression efficiency checks and will store data uncompressed if compression
// doesn't provide sufficient space savings after accounting for header overhead.
//
// The context allows for cancellation of long-running compression operations.
// Returns compressed data with metadata headers or an error if compression fails or exceeds limits.
func (c *GzipCompressor) Compress(ctx context.Context, data []byte) ([]byte, error) {
	// For small data, compression is fast - no need for context checks
	if len(data) <= largePayloadThreshold {
		return c.compressSmall(data)
	}

	// Check context before starting expensive compression
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// For large payloads, use context-aware compression
	return c.compressLargePayload(ctx, data)
}

// Decompress decompresses cached data, automatically detecting the compression format from headers.
// The method handles both compressed (gzip) and uncompressed data transparently.
// Large payloads use streaming decompression to control memory usage and prevent exhaustion.
//
// The method validates magic bytes and version information for data integrity.
// Memory limits are enforced during decompression to prevent memory exhaustion attacks.
//
// The context allows for cancellation of long-running decompression operations.
// Returns the original uncompressed data or an error if decompression fails or exceeds limits.
func (c *GzipCompressor) Decompress(ctx context.Context, data []byte) ([]byte, error) {
	// Check context before starting decompression
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Parse header to determine expected size
	result, err := decodeCompressedData(data)
	if err != nil {
		// Fallback: assume its legacy uncompressed data without header
		return data, nil
	}

	// Handle uncompressed data (when compression flag is false)
	if !result.Metadata.IsCompressed {
		return result.Data, nil
	}

	expectedSize := result.Metadata.OriginalSize

	// For small expected output, decompression is fast - use standard method
	if expectedSize <= largePayloadThreshold {
		return c.decompressSmall(ctx, data, result)
	}

	// For large expected output, use context-aware decompression
	return c.decompressLarge(ctx, result)
}
