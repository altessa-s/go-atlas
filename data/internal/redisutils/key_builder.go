// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisutils

import "strings"

const (
	// DefaultSeparator is the standard Redis key separator.
	DefaultSeparator = ":"
)

// KeyBuilder provides thread-safe, efficient Redis key construction with prefix support.
// The prefix is normalized once during creation to ensure consistent key format.
type KeyBuilder struct {
	prefix    string
	separator string
}

// NewKeyBuilder creates a KeyBuilder with the given prefix.
// The prefix is normalized: trailing separators are removed and will be added
// automatically when building keys.
//
// Example:
//
//	kb := NewKeyBuilder("myapp:cache")  // or "myapp:cache:"
//	kb.Build("user:123") // returns "myapp:cache:user:123"
func NewKeyBuilder(prefix string) *KeyBuilder {
	return NewKeyBuilderWithSeparator(prefix, DefaultSeparator)
}

// NewKeyBuilderWithSeparator creates a KeyBuilder with custom separator.
// Use this when you need a different separator (rare case).
func NewKeyBuilderWithSeparator(prefix, separator string) *KeyBuilder {
	// Normalize prefix: remove trailing separator if present
	prefix = strings.TrimSuffix(prefix, separator)

	return &KeyBuilder{
		prefix:    prefix,
		separator: separator,
	}
}

// Build constructs a full Redis key by combining prefix and key with separator.
// Returns the key unchanged if no prefix is configured.
// This method is thread-safe and allocation-optimized for common cases.
//
// Example:
//
//	kb.Build("user:123")    // "prefix:user:123"
//	kb.Build("")            // "prefix:"
func (kb *KeyBuilder) Build(key string) string {
	if kb.prefix == "" {
		return key
	}

	// Pre-calculate total length for single allocation
	totalLen := len(kb.prefix) + len(kb.separator) + len(key)

	var b strings.Builder
	b.Grow(totalLen)
	b.WriteString(kb.prefix)
	b.WriteString(kb.separator)
	b.WriteString(key)

	return b.String()
}

// BuildMany constructs multiple Redis keys efficiently.
// Returns a new slice with prefixed keys; original slice is not modified.
func (kb *KeyBuilder) BuildMany(keys []string) []string {
	if kb.prefix == "" {
		// Return a copy to avoid modifying the original slice
		result := make([]string, len(keys))
		copy(result, keys)
		return result
	}

	result := make([]string, len(keys))
	for i, key := range keys {
		result[i] = kb.Build(key)
	}
	return result
}

// Prefix returns the normalized prefix (without trailing separator).
func (kb *KeyBuilder) Prefix() string {
	return kb.prefix
}

// Pattern returns a glob pattern for matching all keys with this prefix.
// Useful for SCAN or KEYS commands.
//
// Example:
//
//	kb.Pattern() // "prefix:*"
func (kb *KeyBuilder) Pattern() string {
	if kb.prefix == "" {
		return "*"
	}
	return kb.prefix + kb.separator + "*"
}

// HasPrefix returns true if a prefix is configured.
func (kb *KeyBuilder) HasPrefix() bool {
	return kb.prefix != ""
}
