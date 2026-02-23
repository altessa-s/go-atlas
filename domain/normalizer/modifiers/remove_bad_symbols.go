// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package modifiers

import (
	"reflect"
	"strings"
	"unicode"
)

const (
	// Unicode code points for deprecated grave and acute clones
	DeprecatedGraveClone = '\u0340'
	DeprecatedAcuteClone = '\u0341'

	// Obsolete Khmer characters
	ObsoleteKhmer1 = '\u17A3'
	ObsoleteKhmer2 = '\u17D3'

	// Line and paragraph separators
	LineSeparator      = '\u2028'
	ParagraphSeparator = '\u2029'

	// BIDI embedding controls
	LeftToRightEmbedding = '\u202A'
	RightToLeftEmbedding = '\u202B'
	PopDirectionalFormat = '\u202C'
	LeftToRightOverride  = '\u202D'
	RightToLeftOverride  = '\u202E'

	// Deprecated symmetric swapping controls
	InhibitSymmetricSwapping  = '\u206A'
	ActivateSymmetricSwapping = '\u206B'

	// Deprecated Arabic form shaping controls
	InhibitArabicFormShaping  = '\u206C'
	ActivateArabicFormShaping = '\u206D'

	// Deprecated national digit shape controls
	InhibitNationalDigitShapes  = '\u206E'
	ActivateNationalDigitShapes = '\u206F'

	// Interlinear annotation characters
	InterlinearAnnotationAnchor     = '\uFFF9'
	InterlinearAnnotationSeparator  = '\uFFFA'
	InterlinearAnnotationTerminator = '\uFFFB'

	// Special characters
	ByteOrderMark              = '\uFEFF'
	ObjectReplacementCharacter = '\uFFFC'

	// Musical notation ranges
	MusicalNotationRangeStart = 0x0001D173
	MusicalNotationRangeEnd   = 0x0001D17A

	// Language tag code point ranges
	LanguageTagRangeStart = 0x000E0000
	LanguageTagRangeEnd   = 0x000E007F
)

// badSymbolLookup is a pre-computed lookup table for bad symbols
// This avoids the overhead of a large switch statement
var badSymbolLookup = map[rune]struct{}{
	// Deprecated Unicode symbols
	DeprecatedGraveClone: {},
	DeprecatedAcuteClone: {},
	ObsoleteKhmer1:       {},
	ObsoleteKhmer2:       {},
	// Separators
	LineSeparator:      {},
	ParagraphSeparator: {},
	// BIDI embedding controls
	LeftToRightEmbedding: {},
	RightToLeftEmbedding: {},
	PopDirectionalFormat: {},
	LeftToRightOverride:  {},
	RightToLeftOverride:  {},
	// Deprecated symmetric swapping controls
	InhibitSymmetricSwapping:  {},
	ActivateSymmetricSwapping: {},
	// Deprecated Arabic form shaping controls
	InhibitArabicFormShaping:  {},
	ActivateArabicFormShaping: {},
	// Deprecated national digit shape controls
	InhibitNationalDigitShapes:  {},
	ActivateNationalDigitShapes: {},
	// Interlinear annotation characters
	InterlinearAnnotationAnchor:     {},
	InterlinearAnnotationSeparator:  {},
	InterlinearAnnotationTerminator: {},
	// Special characters
	ByteOrderMark:              {},
	ObjectReplacementCharacter: {},
}

func init() {
	// Register the modifier
	RegisterModifier("remove_bad_symbols", RemoveBadSymbols)
}

// RemoveBadSymbols removes control characters and deprecated Unicode symbols.
// Returns the original value if no bad symbols are found. Supports string and *string.
//
// Example:
//
//	type Input struct { Text string `normalize:"remove_bad_symbols"` }
func RemoveBadSymbols(v reflect.Value, _ map[string]string) ModifierResult {
	return ApplyStringModifier(
		v,
		func(str string) (string, bool) {
			// Fast-path: check if modification is needed
			if !containsBadSymbols(str) {
				return str, false
			}
			sanitized := sanitizeUnicode(str)
			// Consider it unchanged if result is empty (preserve original)
			if sanitized == "" && str != "" {
				return str, false
			}
			return sanitized, sanitized != str
		},
	)
}

// containsBadSymbols checks if a string contains any bad symbols without allocating.
// Returns true if any control characters or deprecated Unicode symbols are found.
//
// Example:
//
//	if containsBadSymbols(input) { ... }
func containsBadSymbols(s string) bool {
	for _, chr := range s {
		if isBadSymbol(chr) {
			return true
		}
	}
	return false
}

// isBadSymbol checks if a single rune is considered a bad symbol.
// Includes control characters, deprecated Unicode, and special format characters.
func isBadSymbol(chr rune) bool {
	// Check control characters first (most common case)
	if unicode.IsControl(chr) {
		return true
	}

	// Fast lookup for specific bad symbols
	if _, found := badSymbolLookup[chr]; found {
		return true
	}

	// Check ranges for musical notation
	if chr >= MusicalNotationRangeStart && chr <= MusicalNotationRangeEnd {
		return true
	}

	// Check ranges for language tag code points
	if chr >= LanguageTagRangeStart && chr <= LanguageTagRangeEnd {
		return true
	}

	return false
}

func sanitizeUnicode(str string) string {
	return strings.Map(func(chr rune) rune {
		if isBadSymbol(chr) {
			return -1
		}
		return chr
	}, str)
}
