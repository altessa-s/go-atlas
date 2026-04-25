// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package modifiers

import (
	"errors"
	"reflect"

	"github.com/nyaruka/phonenumbers"

	"github.com/altessa-s/go-atlas/core/text/strings"

	lru "github.com/hashicorp/golang-lru/v2"
	stdStrings "strings"
)

const (
	// DefaultPhoneRegion is the ISO 3166-1 alpha-2 country code used for phone
	// number parsing when no "region" parameter is specified in the tag.
	DefaultPhoneRegion = "RU"

	// DefaultPhoneCacheSize is the maximum number of entries in the package-level
	// LRU cache that stores [PhoneCacheEntry] results keyed by input+region.
	DefaultPhoneCacheSize = 10_000
)

// PhoneCacheEntry represents a cached phone normalization result stored in the
// package-level LRU cache. Stores both successful results and error information
// so that repeated calls with the same input avoid re-parsing.
type PhoneCacheEntry struct {
	Result    string // Normalized phone number
	IsError   bool   // Whether normalization resulted in error
	ErrorType error  // The specific error that occurred
}

// Sentinel errors returned by [NormalizePhone] wrapped inside [ModifierError.Cause].
// Use errors.Is to check for specific failure modes.
var (
	// ErrPhoneNoValidDigits is returned by [NormalizePhone] when the input string
	// contains no digit characters after cleaning.
	ErrPhoneNoValidDigits = errors.New("phone number contains no valid digits")
	// ErrPhoneParseFailure is returned by [NormalizePhone] when the phone number
	// could not be parsed by the phonenumbers library for the given region.
	ErrPhoneParseFailure = errors.New("failed to parse phone number")
	// ErrPhoneInvalidForRegion is returned by [NormalizePhone] when the phone number
	// parses successfully but fails validation for the specified region.
	ErrPhoneInvalidForRegion = errors.New("phone number is not valid for region")

	// phoneCache caches normalized phone numbers with LRU eviction.
	phoneCache *lru.Cache[string, *PhoneCacheEntry]
)

func init() {
	var err error
	phoneCache, err = lru.New[string, *PhoneCacheEntry](DefaultPhoneCacheSize)
	if err != nil {
		panic("failed to create phone cache: " + err.Error())
	}
	RegisterModifier("phone", NormalizePhone)
}

// NormalizePhone normalizes phone numbers to international E164 format.
// Uses Russia (RU) as the default region. Returns detailed error on failure.
// Supports the "region" parameter for specifying the default country code.
//
// Example:
//
//	type Contact struct { Phone string `normalize:"phone(region=US)"` }
func NormalizePhone(v reflect.Value, params map[string]string) ModifierResult {
	// Get region parameter or use default
	region := DefaultPhoneRegion
	if regionParam, exists := params["region"]; exists && regionParam != "" {
		// Optimize: Only allocate if not already uppercase (common case: "US", "RU", "GB")
		if strings.IsUppercaseUnsafe(regionParam) {
			region = regionParam
		} else {
			region = stdStrings.ToUpper(regionParam)
		}
	}

	return normalizePhoneWithRegion(v, region)
}

// normalizePhoneWithRegion is the common implementation for phone normalization.
// It handles all the phone number validation, parsing, and formatting logic.
func normalizePhoneWithRegion(v reflect.Value, region string) ModifierResult {
	// Extract string value based on type
	var str string
	var isPointer bool

	switch v.Kind() {
	case reflect.String:
		str = v.String()
		if str == "" {
			return NewModifierResult(v, nil)
		}

	case reflect.Pointer:
		if v.IsNil() {
			return NewModifierResult(v, nil)
		}

		// Handle *string
		if v.Type().Elem().Kind() == reflect.String {
			if strPtr, ok := v.Interface().(*string); ok && strPtr != nil {
				str = *strPtr
				isPointer = true
				if str == "" {
					return NewModifierResult(v, nil)
				}
			} else {
				return NewModifierResult(v, nil)
			}
		} else {
			return NewModifierResult(v, nil)
		}

	default:
		// For other types, return unchanged
		return NewModifierResult(v, nil)
	}

	// Clean the input string
	cleaned := cleanPhoneNumber(str)
	if cleaned == "" {
		// Cache the error result using original string as key
		setCachedPhoneResult(str, region, "", ErrPhoneNoValidDigits)
		return NewModifierResult(v, &ModifierError{
			ModifierName:  "phone",
			OriginalValue: str,
			Cause:         ErrPhoneNoValidDigits,
		})
	}

	// Check cache first for performance optimization
	// Try both original and cleaned string as keys
	if result := handleCachedResult(str, region, str, v, isPointer); result != nil {
		return *result
	}

	// Also check cleaned string if different from original
	if cleaned != str {
		if result := handleCachedResult(cleaned, region, str, v, isPointer); result != nil {
			return *result
		}
	}

	// Try to parse the phone number with the specified region
	parsed, err := phonenumbers.Parse(cleaned, region)
	if err != nil {
		// Cache the error result
		setCachedPhoneResult(cleaned, region, "", ErrPhoneParseFailure)
		return NewModifierResult(v, &ModifierError{
			ModifierName:  "phone",
			OriginalValue: str,
			Cause:         ErrPhoneParseFailure,
		})
	}

	// Validate the phone number
	if !phonenumbers.IsValidNumber(parsed) {
		// Cache the error result
		setCachedPhoneResult(cleaned, region, "", ErrPhoneInvalidForRegion)
		return NewModifierResult(v, &ModifierError{
			ModifierName:  "phone",
			OriginalValue: str,
			Cause:         ErrPhoneInvalidForRegion,
		})
	}

	// Format to international E164 format
	formatted := phonenumbers.Format(parsed, phonenumbers.E164)

	// Cache the successful result
	setCachedPhoneResult(cleaned, region, formatted, nil)

	// Return the formatted result
	return createResultFromString(formatted, str, v, isPointer)
}

// createResultFromString creates ModifierResult from a formatted string.
// Handles both pointer and non-pointer cases with zero-copy optimization.
func createResultFromString(formatted, original string, originalValue reflect.Value, isPointer bool) ModifierResult {
	// Zero-copy optimization: check if result is the same as input
	if formatted == original {
		return NewModifierResult(originalValue, nil)
	}

	if isPointer {
		return NewModifierResult(reflect.ValueOf(&formatted), nil)
	}
	return NewModifierResult(reflect.ValueOf(formatted), nil)
}

// handleCachedResult processes cached phone results and returns appropriate ModifierResult.
// Returns nil if no cached entry found.
func handleCachedResult(cacheKey, region, originalStr string, originalValue reflect.Value, isPointer bool) *ModifierResult {
	cached := getCachedPhoneResult(cacheKey, region)
	if cached == nil {
		return nil
	}

	if cached.IsError {
		result := NewModifierResult(originalValue, &ModifierError{
			ModifierName:  "phone",
			OriginalValue: originalStr,
			Cause:         cached.ErrorType,
		})
		return &result
	}

	// Return cached successful result
	result := createResultFromString(cached.Result, originalStr, originalValue, isPointer)
	return &result
}

// cleanPhoneNumber removes common non-digit characters from phone number string
// but preserves the leading + for international format detection.
// Uses single-pass algorithm with optimistic allocation for better performance.
func cleanPhoneNumber(phone string) string {
	if phone == "" {
		return ""
	}

	// Optimistic allocation: most phone numbers have few separators
	// Allocate based on input length, which is usually close to final length
	result := make([]byte, 0, len(phone))

	for i, r := range phone {
		switch {
		case r == '+' && i == 0:
			// Keep leading plus sign
			result = append(result, '+')
		case r >= '0' && r <= '9':
			// Keep digits - direct byte conversion for ASCII digits
			result = append(result, byte(r))
		case r == ' ' || r == '-' || r == '(' || r == ')' || r == '.' || r == '_':
			// Skip common separators
			continue
		default:
			// Skip other characters
			continue
		}
	}

	if len(result) == 0 {
		return ""
	}

	return string(result)
}

// buildPhoneCacheKey creates a cache key for phone normalization.
// Combines input string and region for unique identification.
func buildPhoneCacheKey(input, region string) string {
	if region == "" {
		region = DefaultPhoneRegion
	}

	// Direct concatenation is often faster for small strings
	// and avoids the overhead of strings.Builder
	return input + "|" + region
}

// getCachedPhoneResult retrieves a cached phone normalization result.
// Returns nil if not found in cache.
func getCachedPhoneResult(input, region string) *PhoneCacheEntry {
	key := buildPhoneCacheKey(input, region)
	entry, ok := phoneCache.Get(key)
	if ok {
		return entry
	}
	return nil
}

// setCachedPhoneResult stores a phone normalization result in cache.
func setCachedPhoneResult(input, region, result string, err error) {
	key := buildPhoneCacheKey(input, region)
	entry := &PhoneCacheEntry{
		Result:    result,
		IsError:   err != nil,
		ErrorType: err,
	}
	phoneCache.Add(key, entry)
}
