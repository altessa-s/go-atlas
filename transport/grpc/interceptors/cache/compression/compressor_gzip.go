// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package compression

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// compressSmall handles compression of small payloads without context checking
func (c *GzipCompressor) compressSmall(data []byte) ([]byte, error) {
	// Check payload size limits
	if len(data) > maxPayloadSize {
		return nil, ErrPayloadTooLarge
	}

	if !c.ShouldCompress(data) {
		// Return uncompressed data with proper header for small data
		return encodeUncompressedData(data)
	}

	// Always use gzip compression since this is a gzip-only Compressor
	compressed, err := c.doCompress(data)

	if err != nil {
		return nil, coreerrs.Wrap(err, "compression failed")
	}

	// Check if compression actually reduced size (with header overhead consideration)
	headerOverhead := headerSize
	if len(compressed)+headerOverhead >= len(data) {
		// Return original data with uncompressed encoding if compression didn't help
		return encodeUncompressedData(data)
	}

	// Wrap compressed data with metadata
	result := compressedData{
		Metadata: Metadata{
			IsCompressed: true,
			OriginalSize: len(data),
		},
		Data: compressed,
	}

	return encodeCompressedData(result)
}

// decompressSmall handles decompression of small payloads
func (c *GzipCompressor) decompressSmall(_ context.Context, data []byte, result compressedData) ([]byte, error) {
	// Check payload size limits
	if len(data) > maxPayloadSize {
		return nil, ErrPayloadTooLarge
	}

	expectedSize := result.Metadata.OriginalSize

	// Security: Validate expectedSize to prevent resource exhaustion attacks
	if expectedSize < 0 {
		return nil, fmt.Errorf("invalid negative original size: %d", expectedSize)
	}

	if expectedSize > maxDecompressionSize {
		return nil, fmt.Errorf("decompressed size %d exceeds maximum allowed size %d",
			expectedSize, maxDecompressionSize)
	}

	// Multi-layered security validation against zip bomb attacks
	compressedSize := len(result.Data)
	if compressedSize > 0 {
		// Layer 1: Compression ratio validation with size-based thresholds
		// Note: expectedSize is already validated against maxDecompressionSize above
		compressionRatio := float64(expectedSize) / float64(compressedSize)

		// Apply stricter limits for small payloads that compress suspiciously well
		maxAllowedRatio := maxCompressionRatio
		if compressedSize < suspiciousPayloadThreshold {
			maxAllowedRatio = maxSuspiciousCompressionRatio
		}

		if compressionRatio > float64(maxAllowedRatio) {
			return nil, fmt.Errorf("suspicious compression ratio: %d compressed to %d bytes (ratio: %.1f:1, max allowed: %d:1 for payload size %d)",
				compressedSize, expectedSize, compressionRatio, maxAllowedRatio, compressedSize)
		}

		// Layer 3: Sanity check for reasonable compression ratios
		// Even legitimate data shouldn't compress better than 1000:1 under any circumstances
		const AbsoluteMaxCompressionRatio = 1000
		if compressionRatio > AbsoluteMaxCompressionRatio {
			return nil, fmt.Errorf("impossible compression ratio: %.1f:1 exceeds physical limits", compressionRatio)
		}
	}

	// Decompress using gzip (only supported compression)
	if result.Metadata.IsCompressed {
		return c.doDecompress(result.Data)
	}

	// Should not reach here as we already handled uncompressed data above
	return nil, fmt.Errorf("invalid compression state: data marked as compressed but not handled")
}

// streamDecompress performs streaming decompression with memory control and context cancellation support
func (c *GzipCompressor) streamDecompress(ctx context.Context, reader *gzip.Reader, expectedSize int) ([]byte, error) {
	// Security: Additional validation of expectedSize
	if expectedSize < 0 || expectedSize > maxDecompressionSize {
		return nil, fmt.Errorf("invalid expected size for decompression: %d", expectedSize)
	}

	// The accumulator is returned to the caller, so it must be caller-owned and
	// never drawn from a pool. A pooled backing array would be handed back to the
	// pool (here or during growth) while the returned slice still aliases it,
	// leaking one request's decompressed payload into the next.
	result := make([]byte, 0, expectedSize)

	// Use optimally sized streaming buffer
	streamingBufSize := getOptimalStreamingBufferSize(expectedSize)
	buf := make([]byte, streamingBufSize)

	for {
		// Check context before each read operation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		n, err := reader.Read(buf)
		if n > 0 {
			growthResult := c.appendWithGrowth(result, buf[:n], expectedSize)
			if growthResult.err != nil {
				// Return specific error instead of generic message
				return nil, coreerrs.Wrap(growthResult.err, "buffer growth failed during decompression")
			}
			result = growthResult.data
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, coreerrs.Wrap(err, "decompression read failed")
		}
	}

	return result, nil
}

// appendWithGrowthResult represents the result of buffer growth operation
type appendWithGrowthResult struct {
	data []byte
	err  error
}

// Common errors for buffer growth operations
var (
	ErrBufferSizeExceeded = fmt.Errorf("buffer size exceeded maximum allowed limit")
	ErrBufferOverflow     = fmt.Errorf("buffer capacity overflow detected")
)

// appendWithGrowth appends data to result with controlled growth and size checking
// Returns error information to distinguish between different failure modes
func (c *GzipCompressor) appendWithGrowth(result, newData []byte, expectedSize int) appendWithGrowthResult {
	newSize := len(result) + len(newData)

	// Validate size constraints
	if err := c.validateBufferSize(newSize, expectedSize); err != nil {
		return appendWithGrowthResult{nil, err}
	}

	// Grow buffer if needed
	if newSize > cap(result) {
		newResult, err := c.growBuffer(result, newSize)
		if err != nil {
			return appendWithGrowthResult{nil, err}
		}
		result = newResult
	}

	return appendWithGrowthResult{append(result, newData...), nil}
}

// validateBufferSize checks if the new buffer size is within acceptable limits
func (c *GzipCompressor) validateBufferSize(newSize, expectedSize int) error {
	// Safety check: don't exceed expected size by too much
	if newSize > expectedSize*maxDecompressionSizeMultiplier {
		return ErrBufferSizeExceeded
	}
	return nil
}

// growBuffer grows the buffer capacity using direct allocation. The grown slice
// propagates up to the caller of streamDecompress, so it must never be backed by
// a pooled array: a pooled result would alias a buffer that is later reused for
// another request, exposing one request's data to the next.
func (c *GzipCompressor) growBuffer(result []byte, newSize int) ([]byte, error) {
	newCapacity, err := c.calculateBufferCapacity(newSize)
	if err != nil {
		return nil, err
	}
	return c.growBufferDirect(result, newCapacity), nil
}

// calculateBufferCapacity determines the optimal buffer capacity with safety checks
func (c *GzipCompressor) calculateBufferCapacity(newSize int) (int, error) {
	newCapacity := newSize * bufferGrowthMultiplier

	// Enforce maximum buffer size with safety margin
	maxAllowedCapacity := maxBufferSize - bufferSafetyMargin
	if newCapacity > maxAllowedCapacity {
		if newSize <= maxAllowedCapacity {
			newCapacity = maxAllowedCapacity
		} else {
			return 0, ErrBufferSizeExceeded
		}
	}

	// Validate capacity calculation
	if newCapacity < newSize {
		return 0, ErrBufferOverflow
	}

	return newCapacity, nil
}

// growBufferDirect grows buffer using direct memory allocation
func (c *GzipCompressor) growBufferDirect(result []byte, newCapacity int) []byte {
	newResultBuffer := make([]byte, len(result), newCapacity)
	copy(newResultBuffer, result)
	return newResultBuffer
}

func (c *GzipCompressor) doCompress(data []byte) ([]byte, error) {
	buf := getBytesBuffer()
	defer putBytesBuffer(buf)

	writer, err := getGzipWriter(c.compressionLevel)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "get gzip writer")
	}

	// Ensure proper cleanup: close before returning to pool
	var writerClosed bool
	defer func() {
		if !writerClosed {
			_ = writer.Close() // Ensure writer is closed before returning to pool
		}
		putGzipWriter(writer, c.compressionLevel)
	}()

	writer.Reset(buf)

	if _, err := writer.Write(data); err != nil {
		return nil, coreerrs.WrapOperation(err, "write compressed data")
	}

	if err := writer.Close(); err != nil {
		return nil, coreerrs.WrapOperation(err, "close gzip writer")
	}
	writerClosed = true

	// Avoid copy by returning buffer bytes directly (zero-copy optimization)
	resultBytes := buf.Bytes()
	if len(resultBytes) == 0 {
		return nil, fmt.Errorf("compression resulted in empty buffer")
	}

	// For pooled buffers, we need to copy to prevent data corruption
	// when the buffer is returned to pool and reused
	result := make([]byte, len(resultBytes))
	copy(result, resultBytes)
	return result, nil
}

func (c *GzipCompressor) doDecompress(data []byte) ([]byte, error) {
	// For very large data, use streaming to avoid large memory allocations
	if len(data) > maxPooledBufferSize {
		return c.doDecompressStreaming(data)
	}

	pooledReader := getGzipReader()
	defer putGzipReader(pooledReader) // This will handle reader.Close() properly

	var err error
	if pooledReader.reader == nil {
		pooledReader.reader, err = gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "create gzip reader")
		}
	} else {
		err = pooledReader.reader.Reset(bytes.NewReader(data))
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "reset gzip reader")
		}
	}

	result, err := io.ReadAll(pooledReader.reader)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "read decompressed data")
	}
	return result, nil
}

func (c *GzipCompressor) doDecompressStreaming(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()

	// Pre-allocate result buffer to avoid growing slice
	// Use a reasonable size based on typical compression ratios
	estimatedSize := len(data) * decompressionRatioEstimate // Assume expansion after decompression
	result := make([]byte, 0, estimatedSize)

	// Use appropriately sized buffer for reading chunks
	buf := getSizedBytesBuffer(estimatedSize)
	defer putSizedBytesBuffer(buf, estimatedSize)

	// Read in chunks to avoid large intermediate allocations
	for {
		n, err := reader.Read(buf.Bytes()[:cap(buf.Bytes())])
		if n > 0 {
			result = append(result, buf.Bytes()[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		buf.Reset()
	}

	return result, nil
}

// compressLargePayload performs context-aware compression for large payloads.
// It periodically checks the context for cancellation during the compression process.
func (c *GzipCompressor) compressLargePayload(ctx context.Context, data []byte) ([]byte, error) {
	// Check payload size limits
	if len(data) > maxPayloadSize {
		return nil, ErrPayloadTooLarge
	}

	if !c.ShouldCompress(data) {
		return encodeUncompressedData(data)
	}

	// Create output buffer with compression ratio estimation
	estimatedSize := len(data) / compressionRatioEstimate
	buf := getBytesBuffer()
	defer putBytesBuffer(buf)

	// Pre-grow buffer to estimated size to reduce allocations
	buf.Grow(estimatedSize + headerSize)

	// Use context-aware streaming compression
	if err := c.doCompressStreaming(ctx, data, buf); err != nil {
		return nil, coreerrs.Wrap(err, "streaming compression failed")
	}

	// Check if compression was effective
	compressedSize := buf.Len()
	if compressedSize >= len(data)-headerSize {
		// Compression not effective, return uncompressed
		return encodeUncompressedData(data)
	}

	// Return compressed data with metadata
	compressedEntry := compressedData{
		Metadata: Metadata{
			IsCompressed: true,
			OriginalSize: len(data),
		},
		Data: buf.Bytes(),
	}

	return encodeCompressedData(compressedEntry)
}

// doCompressStreaming performs streaming compression with context checks.
// It periodically checks for context cancellation during the compression process.
func (c *GzipCompressor) doCompressStreaming(ctx context.Context, data []byte, output *bytes.Buffer) error {
	writer, err := getGzipWriter(c.compressionLevel)
	if err != nil {
		return coreerrs.WrapOperation(err, "get gzip writer")
	}

	// Ensure proper cleanup: close before returning to pool, even on context cancel
	var writerClosed bool
	defer func() {
		if !writerClosed {
			_ = writer.Close() // Ensure writer is closed before returning to pool
		}
		putGzipWriter(writer, c.compressionLevel)
	}()

	writer.Reset(output)

	// Write data in optimally-sized chunks with context checking
	chunkSize := getOptimalStreamingBufferSize(len(data))
	for i := 0; i < len(data); i += chunkSize {
		// Check context periodically during long operations
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		end := min(i+chunkSize, len(data))

		if _, err := writer.Write(data[i:end]); err != nil {
			return coreerrs.Wrap(err, "compression write failed")
		}
	}

	// Final context check before closing
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if err := writer.Close(); err != nil {
		return coreerrs.WrapOperation(err, "close gzip writer")
	}
	writerClosed = true
	return nil
}

// decompressLarge performs context-aware decompression for large payloads.
func (c *GzipCompressor) decompressLarge(ctx context.Context, result compressedData) ([]byte, error) {
	expectedSize := result.Metadata.OriginalSize

	// Multi-layered security validation with context
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Apply existing security checks
	compressedSize := len(result.Data)
	if compressedSize > 0 {
		// Note: expectedSize is already validated against maxDecompressionSize above
		compressionRatio := float64(expectedSize) / float64(compressedSize)
		maxAllowedRatio := maxCompressionRatio
		if compressedSize < suspiciousPayloadThreshold {
			maxAllowedRatio = maxSuspiciousCompressionRatio
		}

		if compressionRatio > float64(maxAllowedRatio) {
			return nil, fmt.Errorf("suspicious compression ratio: %d compressed to %d bytes (ratio: %.1f:1, max allowed: %d:1 for payload size %d)",
				compressedSize, expectedSize, compressionRatio, maxAllowedRatio, compressedSize)
		}
	}

	// Perform context-aware streaming decompression
	return c.streamDecompressFromData(ctx, result.Data, expectedSize)
}

// streamDecompressFromData performs streaming decompression from raw data with context cancellation support
func (c *GzipCompressor) streamDecompressFromData(ctx context.Context, data []byte, expectedSize int) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "create gzip reader")
	}
	defer func() { _ = reader.Close() }()

	return c.streamDecompress(ctx, reader, expectedSize)
}
