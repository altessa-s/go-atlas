// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package validation

// UUIDLength is the standard UUID v4 string length (8-4-4-4-12 format).
const UUIDLength = 36

// IsValidUUIDv4 reports whether s is a well-formed UUID version 4 string
// in the canonical 8-4-4-4-12 format (e.g. "550e8400-e29b-41d4-a716-446655440000").
//
// It validates:
//   - total length ([UUIDLength] = 36)
//   - hyphen positions (8, 13, 18, 23)
//   - version nibble (must be '4')
//   - variant nibble (must be 8, 9, a, or b, case-insensitive)
//   - all remaining characters are hexadecimal
//
// The check is performed byte-by-byte without regexp to minimize
// allocations on hot request paths.
func IsValidUUIDv4(s string) bool {
	if len(s) != UUIDLength {
		return false
	}

	// Check format: 8-4-4-4-12 (hyphen positions)
	if s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return false
	}

	// Check version (must be 4)
	if s[14] != '4' {
		return false
	}

	// Check variant (must be 8, 9, a, or b)
	variant := s[19]
	if variant != '8' && variant != '9' && variant != 'a' && variant != 'b' &&
		variant != 'A' && variant != 'B' {
		return false
	}

	// Validate hex characters
	for i := range UUIDLength {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			continue // Skip hyphens
		}
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}

	return true
}
