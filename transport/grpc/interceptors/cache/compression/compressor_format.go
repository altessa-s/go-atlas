// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package compression

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sync"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Header creation helper functions
func createCompressedHeader(originalSize int) OptimizedHeader {
	return OptimizedHeader{
		Magic:        magicBytes,
		Version:      versionByte,
		Reserved:     0,                    // Reserved for future use
		OriginalSize: uint32(originalSize), //nolint:gosec // G115: size bounded by gRPC message limits (< 4GB)
		IsCompressed: compressedFlag,       // 1 indicates data is gzip compressed
	}
}

func createUncompressedHeader(originalSize int) OptimizedHeader {
	return OptimizedHeader{
		Magic:        magicBytes,
		Version:      versionByte,
		Reserved:     0,                    // Reserved for future use
		OriginalSize: uint32(originalSize), //nolint:gosec // G115: size bounded by gRPC message limits (< 4GB)
		IsCompressed: uncompressedFlag,     // 0 indicates data is not compressed
	}
}

// Common encoding helper function with optimized buffer management
func encodeDataWithHeader(header OptimizedHeader, data []byte, totalSize int) ([]byte, error) {
	// For very small data, use zero-copy direct allocation
	if len(data) <= smallDataThreshold {
		return encodeDataZeroCopy(header, data, totalSize)
	}

	// For larger data, use appropriately sized pooled buffers
	buf := getSizedBytesBuffer(totalSize)
	defer putSizedBytesBuffer(buf, totalSize)

	// Pre-grow buffer to exact size to avoid reallocations
	buf.Grow(totalSize)

	// Encode header directly into buffer to avoid intermediate allocation
	if buf.Len() == 0 {
		// Pre-allocate space for header and data
		initialBytes := make([]byte, 0, totalSize)
		buf = bytes.NewBuffer(initialBytes)
	}

	// Zero-copy header encoding directly into buffer
	headerStart := buf.Len()
	buf.Grow(headerSize)
	bufBytes := buf.Bytes()
	if err := encodeHeaderZeroCopy(header, bufBytes[headerStart:headerStart+headerSize]); err != nil {
		return nil, coreerrs.WrapOperation(err, "encode header")
	}
	// Update buffer length to include header
	buf = bytes.NewBuffer(bufBytes[:headerStart+headerSize])

	// Append data directly
	buf.Write(data)

	// Return slice of buffer data to avoid copy
	return buf.Bytes(), nil
}

// Zero-copy encoding for small to large data with pooled slice reuse
func encodeDataZeroCopy(header OptimizedHeader, data []byte, totalSize int) ([]byte, error) {
	// The encoded buffer is returned to the caller and written to the response,
	// so it must be caller-owned. It must not be drawn from a pool: a pooled slice
	// that escapes here is never safely returned and, once reused, would alias
	// another request's data.
	result := make([]byte, totalSize)

	// Zero-copy header encoding directly into result buffer
	if err := encodeHeaderZeroCopy(header, result[:headerSize]); err != nil {
		return nil, coreerrs.WrapOperation(err, "encode header")
	}

	// Zero-copy data append using direct memory copy
	copy(result[headerSize:], data)

	return result, nil
}

// Zero-copy header encoding functions
func encodeHeaderZeroCopy(header OptimizedHeader, buf []byte) error {
	// Ensure buffer is large enough
	if len(buf) < headerSize {
		return fmt.Errorf("buffer too small for header: got %d bytes, need %d", len(buf), headerSize)
	}

	// Zero-copy encoding using named offsets
	// This avoids binary.Write overhead and intermediate allocations
	binary.BigEndian.PutUint16(buf[magicOffset:magicOffset+2], header.Magic)
	buf[versionOffset] = header.Version
	buf[reservedOffset] = header.Reserved
	binary.BigEndian.PutUint32(buf[originalSizeOffset:originalSizeOffset+4], header.OriginalSize)
	buf[compressedOffset] = header.IsCompressed

	return nil
}

func decodeHeaderZeroCopy(data []byte) OptimizedHeader {
	// Direct memory access without intermediate allocations using named offsets
	return OptimizedHeader{
		Magic:        binary.BigEndian.Uint16(data[magicOffset : magicOffset+2]),
		Version:      data[versionOffset],
		Reserved:     data[reservedOffset],
		OriginalSize: binary.BigEndian.Uint32(data[originalSizeOffset : originalSizeOffset+4]),
		IsCompressed: data[compressedOffset],
	}
}

// encodeCompressedData encodes compressed data with metadata using optimized binary format and buffer pooling.
// Format: [magic(2)][version(1)][algorithm(1)][original_size(4)][compressed_flag(1)][data...]
func encodeCompressedData(data compressedData) ([]byte, error) {
	totalSize := headerSize + len(data.Data)
	header := createCompressedHeader(data.Metadata.OriginalSize)
	return encodeDataWithHeader(header, data.Data, totalSize)
}

// decodeCompressedData decodes compressed data with metadata using optimized binary format.
func decodeCompressedData(data []byte) (compressedData, error) {
	// Validate basic data format
	if err := validateDataFormat(data); err != nil {
		return compressedData{}, err
	}

	// Decode and validate header
	header, err := decodeAndValidateHeader(data)
	if err != nil {
		return compressedData{}, err
	}

	// Extract payload data with optimal memory strategy
	resultData := extractPayloadData(data)

	return compressedData{
		Metadata: Metadata{
			IsCompressed: header.IsCompressed == compressedFlag,
			OriginalSize: int(header.OriginalSize),
		},
		Data: resultData,
	}, nil
}

// validateDataFormat performs basic validation of compressed data format
func validateDataFormat(data []byte) error {
	if len(data) < headerSize {
		return fmt.Errorf("invalid compressed data format: too short (%d bytes)", len(data))
	}

	// Fast magic number validation (avoid full header parsing for invalid data)
	magic := binary.BigEndian.Uint16(data[magicOffset : magicOffset+2])
	if magic != magicBytes {
		return fmt.Errorf("invalid magic bytes: 0x%04X", magic)
	}

	return nil
}

// decodeAndValidateHeader decodes header and validates all header fields
func decodeAndValidateHeader(data []byte) (OptimizedHeader, error) {
	// Zero-copy header decoding (avoids binary.Read overhead)
	header := decodeHeaderZeroCopy(data[:headerSize])

	// Validate version
	if header.Version != versionByte {
		return header, fmt.Errorf("unsupported compression version: %d", header.Version)
	}

	// Check if compression flag is valid (0 or 1)
	if header.IsCompressed != compressedFlag && header.IsCompressed != uncompressedFlag {
		return header, fmt.Errorf("invalid compression flag: %d", header.IsCompressed)
	}

	return header, nil
}

// extractPayloadData extracts payload using optimal memory strategy based on data size
func extractPayloadData(data []byte) []byte {
	expectedDataSize := len(data) - headerSize
	if expectedDataSize < 0 {
		return nil
	}

	// Use zero-copy slice when safe, copy only when necessary for memory efficiency
	if expectedDataSize <= smallDataThreshold {
		// For small data, zero-copy slice is safe and efficient
		return data[headerSize:]
	}

	// For larger data, copy to prevent memory retention issues where
	// a small slice keeps a large underlying array alive
	resultData := make([]byte, expectedDataSize)
	copy(resultData, data[headerSize:])
	return resultData
}

// encodeUncompressedData encodes uncompressed data with minimal overhead and buffer pooling.
// Format: [magic(2)][version(1)][algorithm=0(1)][original_size(4)][compressed_flag=0(1)][data...]
func encodeUncompressedData(data []byte) ([]byte, error) {
	totalSize := headerSize + len(data)
	header := createUncompressedHeader(len(data))
	return encodeDataWithHeader(header, data, totalSize)
}

// getSizedBytesBuffer returns an appropriately sized bytes.Buffer from the pool
func getSizedBytesBuffer(expectedSize int) *bytes.Buffer {
	var pool *sync.Pool

	switch {
	case expectedSize <= SmallBufferSize:
		pool = &smallBufferPool
	case expectedSize <= MediumBufferSize:
		pool = &mediumBufferPool
	default:
		pool = &largeBufferPool
	}

	buf, ok := pool.Get().(*bytes.Buffer)
	if !ok {
		// Fallback: create new buffer if pool returns unexpected type
		capacity := getOptimalBufferCapacity(expectedSize)
		buf = bytes.NewBuffer(make([]byte, 0, capacity))
	} else {
		buf.Reset()
	}
	return buf
}

// putSizedBytesBuffer returns a bytes.Buffer to the appropriate pool
func putSizedBytesBuffer(buf *bytes.Buffer, originalSize int) {
	// Only return to pool if size is reasonable
	if buf.Cap() <= maxPooledBufferSize {
		var pool *sync.Pool

		switch {
		case originalSize <= SmallBufferSize:
			pool = &smallBufferPool
		case originalSize <= MediumBufferSize:
			pool = &mediumBufferPool
		default:
			pool = &largeBufferPool
		}

		pool.Put(buf)
	}
}

// getOptimalBufferCapacity calculates optimal buffer capacity for given size
func getOptimalBufferCapacity(expectedSize int) int {
	switch {
	case expectedSize <= SmallBufferSize:
		return SmallBufferSize
	case expectedSize <= MediumBufferSize:
		return MediumBufferSize
	case expectedSize <= LargeBufferSize:
		return LargeBufferSize
	default:
		// For very large sizes, use next power of 2 or max pool size
		capacity := 1
		for capacity < expectedSize && capacity < maxPooledBufferSize {
			capacity <<= 1
		}
		if capacity > maxPooledBufferSize {
			return maxPooledBufferSize
		}
		return capacity
	}
}

// Streaming buffer size constants
const (
	smallStreamingBufferSize     = 4 * 1024   // 4KB for small payloads
	mediumStreamingBufferSize    = 16 * 1024  // 16KB for medium payloads
	largeStreamingBufferSize     = 64 * 1024  // 64KB for large payloads
	veryLargeStreamingBufferSize = 128 * 1024 // 128KB for very large payloads
)

// getOptimalStreamingBufferSize calculates optimal streaming buffer size
func getOptimalStreamingBufferSize(expectedSize int) int {
	switch {
	case expectedSize <= SmallBufferSize:
		return smallStreamingBufferSize
	case expectedSize <= MediumBufferSize:
		return mediumStreamingBufferSize
	case expectedSize <= LargeBufferSize:
		return largeStreamingBufferSize
	default:
		return veryLargeStreamingBufferSize
	}
}
