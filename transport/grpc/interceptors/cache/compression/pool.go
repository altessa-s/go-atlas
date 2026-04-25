// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package compression

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"log/slog"
	"sync"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

var (
	// gzipWriterPools provides reusable gzip writers by compression level using sync.Map for thread safety
	gzipWriterPools sync.Map // map[int]*sync.Pool

	// gzipReaderPool provides reusable gzip readers
	gzipReaderPool = sync.Pool{
		New: func() any {
			return &gzipReader{}
		},
	}
)

// pooled wrapper types for readers to enable pooling
type gzipReader struct {
	reader       *gzip.Reader
	failureCount int // Track consecutive failures to prevent leaky readers from pooling
}

// gzipWriterError represents an error that occurred during writer pool initialization
type gzipWriterError struct {
	err error
}

// Helper functions for backward compatibility buffer management
func getBytesBuffer() *bytes.Buffer {
	// Use the new sized buffer function with default size for backward compatibility
	return getSizedBytesBuffer(defaultBufferSize)
}

func putBytesBuffer(buf *bytes.Buffer) {
	// Use the new sized buffer function with default size for backward compatibility
	putSizedBytesBuffer(buf, defaultBufferSize)
}

func getGzipWriter(level int) (*gzip.Writer, error) {
	pool, err := getWriterPool(level)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "get writer pool")
	}

	poolItem := pool.Get()

	// Check if pool returned an error instead of a writer
	if writerErr, ok := poolItem.(*gzipWriterError); ok {
		return nil, coreerrs.Wrap(writerErr.err, "writer pool error")
	}

	writer, ok := poolItem.(*gzip.Writer)
	if !ok {
		// Fallback: create new writer if pool returns unexpected type
		fallbackWriter, err := gzip.NewWriterLevel(nil, level)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "create fallback gzip writer")
		}
		return fallbackWriter, nil
	}
	return writer, nil
}

// getWriterPool gets or creates a writer pool for the specified compression level
// This function is thread-safe and eliminates race conditions through atomic operations
func getWriterPool(level int) (*sync.Pool, error) {
	// Validate compression level first before any pool operations
	if level < gzip.DefaultCompression || level > gzip.BestCompression {
		return nil, fmt.Errorf("invalid gzip compression level: %d (must be between %d and %d)",
			level, gzip.DefaultCompression, gzip.BestCompression)
	}

	// Create new pool factory function - this will only be called if needed
	createPool := func() *sync.Pool {
		return &sync.Pool{
			New: func() any {
				w, err := gzip.NewWriterLevel(nil, level)
				if err != nil {
					// This should not happen with validated levels, but handle gracefully
					return &gzipWriterError{err: coreerrs.WrapOperation(err, "create gzip writer")}
				}
				return w
			},
		}
	}

	// Atomic operation: either load existing pool or store new one
	// This eliminates the race condition between Load and LoadOrStore
	actual, _ := gzipWriterPools.LoadOrStore(level, createPool())

	// Since we control what goes into the map, this should always succeed
	// If it doesn't, it indicates memory corruption or external interference
	if pool, ok := actual.(*sync.Pool); ok {
		return pool, nil
	}

	// This should never happen in normal operation - indicates serious corruption
	return nil, fmt.Errorf("critical error: corrupted pool state for level %d (type: %T)", level, actual)
}

func putGzipWriter(w *gzip.Writer, level int) {
	w.Reset(nil)
	if pool, ok := gzipWriterPools.Load(level); ok {
		if typedPool, ok := pool.(*sync.Pool); ok {
			typedPool.Put(w)
		}
	}
}

func getGzipReader() *gzipReader {
	reader, ok := gzipReaderPool.Get().(*gzipReader)
	if !ok {
		// Fallback: create new reader if pool returns unexpected type
		reader = &gzipReader{}
	}
	return reader
}

const maxReaderFailures = 3 // Maximum consecutive failures before discarding reader

func putGzipReader(r *gzipReader) {
	if r.reader != nil {
		// Log close errors to detect potential resource leaks
		if err := r.reader.Close(); err != nil {
			// Note: We use slog here as this is a utility function without access to interceptor options
			// In production, consider passing logger through context or as parameter
			slog.Warn("compression: failed to close gzip reader",
				"error", err,
				"operation", "putGzipReader")
			r.failureCount++

			// Don't return chronically failing readers to the pool
			if r.failureCount >= maxReaderFailures {
				slog.Warn("compression: discarding gzip reader after consecutive failures",
					"failureCount", r.failureCount,
					"maxAllowed", maxReaderFailures,
					"operation", "putGzipReader")
				r.reader = nil
				return // Don't put back in pool
			}
		} else {
			// Reset failure count on successful close
			r.failureCount = 0
		}
		r.reader = nil
	}

	// Only return to pool if failure count is reasonable
	if r.failureCount < maxReaderFailures {
		gzipReaderPool.Put(r)
	}
}
