// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package masking

import (
	"regexp"
	"strings"
	"sync"
	"unsafe"
)

// byteSlicePool reduces allocations in masking operations.
var byteSlicePool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 64) // reasonable default size
		return &b
	},
}

// PartialMask shows only first and last N characters, masking the rest.
//
// Example:
//
//	mask := masking.PartialMask(2, 2, "*")
//	mask("secret123") // returns "se*****23"
func PartialMask(showFirst, showLast int, maskChar string) MaskFunc {
	if maskChar == "" {
		maskChar = "*"
	}
	maskByte := maskChar[0]

	return func(value string) string {
		length := len(value)
		if length == 0 {
			return value
		}

		// If value is too short, mask entirely
		if length <= showFirst+showLast {
			// For very short strings, avoid pool overhead
			if length <= 8 { //nolint:mnd
				b := make([]byte, length)
				for i := range b {
					b[i] = maskByte
				}
				return string(b)
			}

			// Use pool for longer strings
			bufPtr := byteSlicePool.Get().(*[]byte) //nolint:errcheck
			buf := (*bufPtr)[:0]                    // reset length but keep capacity

			// Fill with mask bytes
			for range length {
				buf = append(buf, maskByte)
			}

			res := string(buf)
			*bufPtr = buf // update pool's slice if reallocated
			byteSlicePool.Put(bufPtr)
			return res
		}

		// Use byte slice from pool
		bufPtr := byteSlicePool.Get().(*[]byte) //nolint:errcheck
		buf := (*bufPtr)[:0]                    // reset length but keep capacity

		// Ensure capacity
		if cap(buf) < length {
			buf = make([]byte, 0, length)
		}

		// Build masked string
		if showFirst > 0 {
			buf = append(buf, value[:showFirst]...)
		}

		maskLen := length - showFirst - showLast
		for range maskLen {
			buf = append(buf, maskByte)
		}

		if showLast > 0 {
			buf = append(buf, value[length-showLast:]...)
		}

		res := string(buf)
		*bufPtr = buf // update pool's slice if reallocated
		byteSlicePool.Put(bufPtr)
		return res
	}
}

// SmartMask shows first 2 and last 2 characters, masking the rest.
//
// Example:
//
//	mask := masking.SmartMask()
//	mask("password123") // returns "pa*******23"
func SmartMask() MaskFunc {
	return PartialMask(2, 2, "*") //nolint:mnd
}

// EmailMask masks email local part while preserving domain.
//
// Example:
//
//	mask := masking.EmailMask()
//	mask("user@example.com") // returns "u****r@example.com"
//
// #nosec G103 -- intentional unsafe for zero-copy string conversion
func EmailMask() MaskFunc {
	return func(value string) string {
		if len(value) == 0 {
			return value
		}

		atIndex := strings.IndexByte(value, '@')
		if atIndex == -1 {
			return FullMask()(value)
		}

		if atIndex <= 2 { //nolint:mnd
			return "***@" + value[atIndex+1:]
		}

		// Use unsafe string building for better performance
		localLen := min(atIndex-2, 5) //nolint:mnd
		result := make([]byte, 0, len(value))
		result = append(result, value[0])
		for range localLen {
			result = append(result, '*')
		}
		result = append(result, value[atIndex-1])
		result = append(result, value[atIndex:]...)

		return unsafe.String(unsafe.SliceData(result), len(result))
	}
}

// PrecomputedMasks caches mask strings by length for performance.
type PrecomputedMasks struct {
	masks map[int]string
	mu    sync.RWMutex
}

// NewPrecomputedMasks creates a cache with pre-computed masks up to length 20.
func NewPrecomputedMasks(maskChar string) *PrecomputedMasks {
	if maskChar == "" {
		maskChar = "*"
	}

	pm := &PrecomputedMasks{
		masks: make(map[int]string),
	}

	// Pre-compute masks for common lengths
	for i := 1; i <= 20; i++ {
		pm.masks[i] = strings.Repeat(maskChar, i)
	}

	return pm
}

// GetMask returns a mask string of asterisks with the given length.
func (pm *PrecomputedMasks) GetMask(length int) string {
	pm.mu.RLock()
	mask, ok := pm.masks[length]
	pm.mu.RUnlock()

	if ok {
		return mask
	}

	// Compute and cache new mask
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Double-check after acquiring write lock
	if mask, ok = pm.masks[length]; ok {
		return mask
	}

	mask = strings.Repeat("*", length)
	// Limit cache size
	if len(pm.masks) < 100 { //nolint:mnd
		pm.masks[length] = mask
	}

	return mask
}

// CachedPartialMask is like PartialMask but uses pre-computed mask strings.
func CachedPartialMask(showFirst, showLast int, maskChar string) MaskFunc {
	pm := NewPrecomputedMasks(maskChar)

	return func(value string) string {
		length := len(value)
		if length == 0 {
			return value
		}

		if length <= showFirst+showLast {
			return pm.GetMask(length)
		}

		var result strings.Builder
		result.Grow(length)

		if showFirst > 0 {
			result.WriteString(value[:showFirst])
		}

		maskLen := length - showFirst - showLast
		result.WriteString(pm.GetMask(maskLen))

		if showLast > 0 {
			result.WriteString(value[length-showLast:])
		}

		return result.String()
	}
}

// FullMask replaces the entire value with 8 asterisks.
//
// Example:
//
//	mask := masking.FullMask()
//	mask("secret") // returns "********"
func FullMask() MaskFunc {
	return func(value string) string {
		if len(value) == 0 {
			return value
		}
		// Always return 8 asterisks for consistency
		return "********"
	}
}

// FixedMask replaces any value with the specified fixed string.
//
// Example:
//
//	mask := masking.FixedMask("[REDACTED]")
//	mask("secret") // returns "[REDACTED]"
func FixedMask(mask string) MaskFunc {
	return func(value string) string {
		if len(value) == 0 {
			return value
		}
		return mask
	}
}

// PatternMask applies mask only if value matches the regex pattern.
func PatternMask(pattern string, mask MaskFunc) MaskFunc {
	re, err := regexp.Compile(pattern)
	return func(value string) string {
		if err != nil || re == nil || mask == nil {
			return value
		}
		if re.MatchString(value) {
			return mask(value)
		}
		return value
	}
}

// HashMask replaces value with "[HASH]" indicator, optionally prefixed.
//
// Example:
//
//	mask := masking.HashMask("user_")
//	mask("secret") // returns "user_[HASH]"
func HashMask(prefix string) MaskFunc {
	return func(value string) string {
		if len(value) == 0 {
			return value
		}
		// Simple hash representation (not cryptographic)
		hash := 0
		for _, ch := range value {
			hash = hash*31 + int(ch) //nolint:mnd
		}
		if prefix != "" {
			return prefix + "[HASH]"
		}
		return "[HASH]"
	}
}

// PhoneMask masks phone numbers, showing first 4 and last 2 characters.
//
// Example:
//
//	mask := masking.PhoneMask()
//	mask("+15551234567") // returns "+155*******67"
func PhoneMask() MaskFunc {
	return func(value string) string {
		if len(value) == 0 {
			return value
		}

		if len(value) < 7 { //nolint:mnd
			return FullMask()(value)
		}

		// Mask all but first 4 and last 2 digits
		return PartialMask(4, 2, "*")(value) //nolint:mnd
	}
}

// CreditCardMask masks card numbers, showing first 4 and last 2 characters.
//
// Example:
//
//	mask := masking.CreditCardMask()
//	mask("4111111111111111") // returns "4111**********11"
func CreditCardMask() MaskFunc {
	return func(value string) string {
		if len(value) == 0 {
			return value
		}

		return PartialMask(4, 2, "*")(value) //nolint:mnd
	}
}
