// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package strings

import (
	"strings"
	"unicode"
)

// ToScreamingSnakeCase converts a CamelCase, PascalCase, or mixed-case string
// to SCREAMING_SNAKE_CASE. Underscores are inserted at word boundaries: before
// an uppercase letter preceded by a lowercase letter (e.g. "aB" becomes "A_B"),
// and before an uppercase letter that is followed by a lowercase letter within
// an uppercase run (e.g. "IDs" becomes "I_DS"). Returns the empty string for
// empty input.
//
// For purely ASCII input a fast byte-level scan is used. If any non-ASCII rune
// is detected, the function falls back to a rune-based conversion for
// correctness.
func ToScreamingSnakeCase(s string) string {
	if s == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(s) + len(s)/2) // Pre-allocate with some head-room for underscores

	for i := range len(s) {
		r := rune(s[i])
		// Handle non-ASCII if present (though identifiers are usually ASCII)
		if r > unicode.MaxASCII {
			// Fallback to rune-based loop for the rest of the string if non-ASCII detected
			return toScreamingSnakeCaseComplex(s)
		}

		if i > 0 && unicode.IsUpper(r) {
			prev := rune(s[i-1])
			// Add underscore if:
			// 1) Previous was lowercase (e.g., "aB" -> "a_B")
			// 2) Current is uppercase but followed by lowercase (e.g., "IDs" -> "I_Ds")
			nextIsLower := i+1 < len(s) && unicode.IsLower(rune(s[i+1]))
			if unicode.IsLower(prev) || (unicode.IsUpper(prev) && nextIsLower) {
				b.WriteByte('_')
			}
		}
		b.WriteRune(unicode.ToUpper(r))
	}

	return b.String()
}

func toScreamingSnakeCaseComplex(s string) string {
	var b strings.Builder
	runes := []rune(s)
	for i := range len(runes) {
		r := runes[i]
		if i > 0 && unicode.IsUpper(r) {
			prevIsLower := unicode.IsLower(runes[i-1])
			nextIsLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if prevIsLower || nextIsLower {
				b.WriteByte('_')
			}
		}
		b.WriteRune(unicode.ToUpper(r))
	}
	return b.String()
}

// ToSnakeCase converts a CamelCase, PascalCase, or mixed-case string to
// snake_case by first applying [ToScreamingSnakeCase] and then lowercasing
// the result. Returns the empty string for empty input.
func ToSnakeCase(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToLower(ToScreamingSnakeCase(s))
}

// ToCamelCase converts a snake_case, SCREAMING_SNAKE_CASE, or space-delimited
// string to camelCase. The first letter is lowercased; each letter following an
// underscore or space is uppercased, and the delimiter itself is removed. Runs
// of uppercase letters within the original input are preserved as-is.
// Returns the empty string for empty input.
func ToCamelCase(s string) string {
	if s == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(s))

	upperNext := false
	first := true

	for i := range len(s) {
		r := rune(s[i])
		if r == '_' || unicode.IsSpace(r) {
			upperNext = true
			continue
		}

		if first {
			b.WriteRune(unicode.ToLower(r))
			first = false
			upperNext = false
			continue
		}

		if upperNext {
			b.WriteRune(unicode.ToUpper(r))
			upperNext = false
		} else {
			// If we were already in upper (acronym), keep it?
			// Standard camelCase: "TestPDF" -> "testPDF" (if acronym preserved) or "testPdf"
			// Our previous implementation preserved acronyms.
			b.WriteRune(r)
		}
	}

	return b.String()
}

// ScreamingSnakeToCamelCase converts a SCREAMING_SNAKE_CASE string to
// camelCase. It delegates directly to [ToCamelCase], which already handles
// underscore-delimited input.
func ScreamingSnakeToCamelCase(s string) string {
	return ToCamelCase(s)
}
