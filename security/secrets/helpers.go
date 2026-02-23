// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets

import (
	"regexp"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

// secretKeyRegex validates that a secret key contains only allowed characters.
// Allows alphanumeric characters, underscores, hyphens, and dots.
// This regex is shared across all providers to ensure consistent key validation.
var secretKeyRegex = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

// ValidateKeyLength efficiently validates key length without trimming if not needed.
func ValidateKeyLength(key string) bool {
	keyLen := len(key)
	if keyLen < MinKeyLength || keyLen > MaxKeyLength {
		return false
	}

	// Check if key is only whitespace (without allocating)
	return !corestrings.IsEmptyOrWhitespace(key)
}

// CreateLockKey efficiently creates a lock key by concatenating provider name and encoded key.
func CreateLockKey(provider, key string) string {
	return corestrings.Concat(provider, ":", key)
}

// ValidateSecretKey validates a secret key according to common rules.
// The key must:
//   - Have length between MinKeyLength and MaxKeyLength
//   - Not be empty or whitespace-only
//   - Contain only alphanumeric characters, underscores, hyphens, and dots
//
// Returns ErrInvalidKey if validation fails.
func ValidateSecretKey(key string) error {
	if !ValidateKeyLength(key) {
		return ErrInvalidKey
	}

	if !secretKeyRegex.MatchString(key) {
		return ErrInvalidKey
	}

	return nil
}
