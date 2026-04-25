// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package compression

import "context"

// NoOpCompressor is a Compressor that does nothing (for testing or when compression is disabled).
type NoOpCompressor struct{}

// NewNoOpCompressor creates a new NoOpCompressor.
func NewNoOpCompressor() *NoOpCompressor {
	return &NoOpCompressor{}
}

// Compress returns the data unchanged.
func (c *NoOpCompressor) Compress(_ context.Context, data []byte) ([]byte, error) {
	return data, nil
}

// Decompress returns the data unchanged.
func (c *NoOpCompressor) Decompress(_ context.Context, data []byte) ([]byte, error) {
	return data, nil
}

// ShouldCompress always returns false.
func (c *NoOpCompressor) ShouldCompress(_ []byte) bool {
	return false
}
