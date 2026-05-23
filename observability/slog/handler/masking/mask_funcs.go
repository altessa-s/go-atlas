// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package masking

import (
	"fmt"
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

// URLMask masks sensitive parts of URLs for safe logging.
// It masks the first path component (typically bucket/container) and shows only
// the last path component (typically file name).
//
// Example:
//
//	mask := masking.URLMask()
//	mask("https://s3.endpoint.com/my-bucket/path/to/file.mp3")
//	// returns "https://s3.endpoint.com/my-***/file.mp3"
//	mask("https://storage.yandexcloud.net/audio-files/operations/123/audio.wav")
//	// returns "https://storage.yandexcloud.net/aud***/audio.wav"
func URLMask() MaskFunc {
	const (
		maskChar           = "*"
		minMaskLen         = 3
		showFirstLen       = 3
		schemeDelimiterLen = 3 // Length of "://"
	)

	maskSuffix := func(s string) string {
		length := len(s)
		if length <= showFirstLen {
			// For very short strings, mask entirely
			return strings.Repeat(maskChar, length)
		}
		prefix := s[:showFirstLen]
		masked := strings.Repeat(maskChar, minMaskLen)
		return prefix + masked
	}

	maskMiddle := func(s string) string {
		length := len(s)
		if length <= showFirstLen*2 {
			return strings.Repeat(maskChar, length)
		}
		prefix := s[:showFirstLen]
		suffix := s[length-showFirstLen:]
		masked := strings.Repeat(maskChar, minMaskLen)
		return prefix + masked + suffix
	}

	return func(value string) string {
		if value == "" {
			return ""
		}

		// Parse the URL
		colonSlash := strings.Index(value, "://")
		if colonSlash < 0 {
			// Not a URL, mask the middle part
			return maskMiddle(value)
		}

		// Find the start of path after authority
		pathStart := strings.IndexByte(value[colonSlash+schemeDelimiterLen:], '/')
		if pathStart < 0 {
			// No path in URL
			return value
		}
		pathStart += colonSlash + schemeDelimiterLen

		// Split path into components
		pathEnd := len(value)
		if queryStart := strings.IndexByte(value[pathStart:], '?'); queryStart >= 0 {
			pathEnd = pathStart + queryStart
		} else if fragStart := strings.IndexByte(value[pathStart:], '#'); fragStart >= 0 {
			pathEnd = pathStart + fragStart
		}

		path := value[pathStart+1 : pathEnd]
		pathParts := strings.Split(path, "/")
		if len(pathParts) == 0 {
			return value
		}

		var sanitizedParts []string

		// First part is usually the bucket/container - mask it
		if len(pathParts) > 0 && pathParts[0] != "" {
			sanitizedParts = append(sanitizedParts, maskSuffix(pathParts[0]))
		}

		// Keep only the last part (file name)
		if len(pathParts) > 1 {
			lastPart := pathParts[len(pathParts)-1]
			if lastPart != "" {
				sanitizedParts = append(sanitizedParts, lastPart)
			}
		}

		// Reconstruct the URL
		result := value[:pathStart+1] + strings.Join(sanitizedParts, "/")
		if pathEnd < len(value) {
			result += value[pathEnd:]
		}
		return result
	}
}

// S3URLMask creates a redacted version of S3/cloud storage URLs suitable for logging.
// It extracts operation IDs if present, otherwise shows only the file name.
//
// Example:
//
//	mask := masking.S3URLMask()
//	mask("https://s3.region.amazonaws.com/bucket/operations/op-123/file.mp3")
//	// returns "s3://.../<op-123>/file.mp3"
//	mask("https://storage.endpoint.com/bucket/some/path/file.mp3")
//	// returns "s3://.../file.mp3"
func S3URLMask() MaskFunc {
	const schemeDelimiterLen = 3 // Length of "://"

	return func(value string) string {
		if value == "" {
			return ""
		}

		// Parse the URL to find path
		colonSlash := strings.Index(value, "://")
		if colonSlash < 0 {
			// Not a URL, return as filename
			if lastSlash := strings.LastIndexByte(value, '/'); lastSlash >= 0 {
				return "s3://.../" + value[lastSlash+1:]
			}
			return "s3://.../" + value
		}

		// Find the start of path after authority
		pathStart := strings.IndexByte(value[colonSlash+schemeDelimiterLen:], '/')
		if pathStart < 0 {
			return "s3://***"
		}
		pathStart += colonSlash + schemeDelimiterLen

		// Extract path without query/fragment
		pathEnd := len(value)
		if queryStart := strings.IndexByte(value[pathStart:], '?'); queryStart >= 0 {
			pathEnd = pathStart + queryStart
		} else if fragStart := strings.IndexByte(value[pathStart:], '#'); fragStart >= 0 {
			pathEnd = pathStart + fragStart
		}

		path := value[pathStart:]
		if pathEnd < len(value) {
			path = value[pathStart:pathEnd]
		}

		// Extract the file name
		fileName := "***"
		if lastSlash := strings.LastIndexByte(path, '/'); lastSlash >= 0 {
			fileName = path[lastSlash+1:]
		} else if path != "/" && path != "" {
			fileName = path
		}

		// Try to extract operation ID from path
		pathParts := strings.Split(strings.Trim(path, "/"), "/")
		for _, part := range pathParts {
			// Look for parts that look like operation IDs
			if strings.HasPrefix(part, "op-") ||
				strings.HasPrefix(part, "operation-") ||
				(len(part) == 36 && strings.Count(part, "-") == 4) { // UUID format
				return "s3://.../<" + part + ">/" + fileName
			}
		}

		// No operation ID found, return minimal info
		return "s3://.../" + fileName
	}
}

func init() {
	// Register simple mask functions
	Register("full", FullMask())
	Register("smart", SmartMask())
	Register("email", EmailMask())
	Register("phone", PhoneMask())
	Register("credit_card", CreditCardMask())
	Register("url", URLMask())
	Register("s3_url", S3URLMask())

	// Aliases for convenience
	Register("cc", CreditCardMask())
	Register("s3", S3URLMask())

	// Register factories for parameterized masks
	RegisterFactory("partial", func(params map[string]any) (MaskFunc, error) {
		showFirst, _ := params["showFirst"].(int)
		showLast, _ := params["showLast"].(int)
		maskChar, _ := params["maskChar"].(string)
		if maskChar == "" {
			maskChar = "*"
		}
		return PartialMask(showFirst, showLast, maskChar), nil
	})

	RegisterFactory("fixed", func(params map[string]any) (MaskFunc, error) {
		value, ok := params["value"].(string)
		if !ok || value == "" {
			return nil, fmt.Errorf("fixed mask requires 'value' parameter")
		}
		return FixedMask(value), nil
	})

	RegisterFactory("pattern", func(params map[string]any) (MaskFunc, error) {
		pattern, ok := params["pattern"].(string)
		if !ok || pattern == "" {
			return nil, fmt.Errorf("pattern mask requires 'pattern' parameter")
		}

		// Compile the regex pattern
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid regex pattern: %w", err)
		}

		// Check if we have a simple replacement string
		if replacement, ok := params["replacement"].(string); ok {
			// Simple regex replacement mode
			return func(value string) string {
				return re.ReplaceAllString(value, replacement)
			}, nil
		}

		// Otherwise use the mask function mode
		replacementType, _ := params["replacementType"].(string)
		if replacementType == "" {
			replacementType = "fixed"
		}

		// Create replacement mask
		var replacementMask MaskFunc
		if replacementType == "fixed" {
			replacementValue, _ := params["replacementValue"].(string)
			if replacementValue == "" {
				replacementValue = "***"
			}
			replacementMask = FixedMask(replacementValue)
		} else {
			// Try to get registered mask
			var ok bool
			replacementMask, ok = Get(replacementType)
			if !ok {
				return nil, fmt.Errorf("unknown replacement mask type: %q", replacementType)
			}
		}

		return PatternMask(pattern, replacementMask), nil
	})

	RegisterFactory("hash", func(params map[string]any) (MaskFunc, error) {
		prefix, _ := params["prefix"].(string)
		return HashMask(prefix), nil
	})

	RegisterFactory("cached_partial", func(params map[string]any) (MaskFunc, error) {
		showFirst, _ := params["showFirst"].(int)
		showLast, _ := params["showLast"].(int)
		maskChar, _ := params["maskChar"].(string)
		if maskChar == "" {
			maskChar = "*"
		}
		return CachedPartialMask(showFirst, showLast, maskChar), nil
	})
}
