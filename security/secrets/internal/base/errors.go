// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package base

import (
	"fmt"

	"github.com/altessa-s/go-atlas/security/secrets"
)

// KeyEncodingError wraps an error with ErrKeyEncoding for consistent error formatting.
// Used when key encoding (e.g., base64) fails during storage operations.
func KeyEncodingError(err error) error {
	return fmt.Errorf("%w: key encoding failed: %w", secrets.ErrKeyEncoding, err)
}

// KeyDecodingError wraps an error with ErrKeyDecoding for consistent error formatting.
// Used when key decoding fails during retrieval operations.
func KeyDecodingError(err error) error {
	return fmt.Errorf("%w: key decoding failed: %w", secrets.ErrKeyDecoding, err)
}

// ValueEncodingError wraps an error with ErrEncoding for consistent error formatting.
// Used when value encoding (e.g., JSON marshaling) fails during save operations.
func ValueEncodingError(err error) error {
	return fmt.Errorf("%w: value encoding failed: %w", secrets.ErrEncoding, err)
}

// ValueDecodingError wraps an error with ErrDecoding for consistent error formatting.
// Used when value decoding (e.g., JSON unmarshaling) fails during retrieval operations.
func ValueDecodingError(err error) error {
	return fmt.Errorf("%w: secret data decoding failed: %w", secrets.ErrDecoding, err)
}
