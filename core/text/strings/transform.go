// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package strings

import (
	"strings"
	"unicode"
	"unicode/utf8"
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

// ToCamelCase converts a snake_case or space-delimited string to camelCase.
// The first letter is lowercased; each letter following an underscore or
// Unicode whitespace is uppercased, and the delimiter itself is removed. All
// other letters keep their original case, so runs of uppercase letters are
// preserved ("SCREAMING_SNAKE" becomes "sCREAMINGSNAKE"); use
// [ScreamingSnakeToCamelCase] for upper-case input. Returns the empty string
// for empty input.
//
// For purely ASCII input a fast byte-level scan is used; otherwise the input
// is processed rune by rune. Invalid UTF-8 bytes are copied through unchanged.
func ToCamelCase(s string) string {
	return toCamelCase(s, false)
}

// ScreamingSnakeToCamelCase converts a SCREAMING_SNAKE_CASE string to
// camelCase: every word is lowercased and each word after the first starts
// with an uppercase letter, so "HELLO_WORLD" becomes "helloWorld" and
// "REAL_IP" becomes "realIp". Delimiters are the same as for [ToCamelCase]
// (underscores and Unicode whitespace). Returns the empty string for empty
// input.
func ScreamingSnakeToCamelCase(s string) string {
	return toCamelCase(s, true)
}

// toCamelCase implements [ToCamelCase] and [ScreamingSnakeToCamelCase]. When
// lowerRest is true, letters that do not start a word are lowercased;
// otherwise they are copied as-is.
func toCamelCase(s string, lowerRest bool) string {
	if s == "" {
		return ""
	}
	if !isASCII(s) {
		return toCamelCaseUnicode(s, lowerRest)
	}

	var b strings.Builder
	b.Grow(len(s))

	upperNext := false
	first := true
	for i := range len(s) {
		c := s[i]
		if c == '_' || IsASCIISpace(c) {
			upperNext = true
			continue
		}
		switch {
		case first:
			c = asciiToLower(c)
		case upperNext:
			c = asciiToUpper(c)
		case lowerRest:
			c = asciiToLower(c)
		}
		first, upperNext = false, false
		b.WriteByte(c)
	}

	return b.String()
}

func toCamelCaseUnicode(s string, lowerRest bool) string {
	var b strings.Builder
	b.Grow(len(s))

	upperNext := false
	first := true
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		raw := s[i : i+size]
		i += size

		if r == '_' || unicode.IsSpace(r) {
			upperNext = true
			continue
		}
		if r == utf8.RuneError && size == 1 {
			// Invalid UTF-8: keep the byte as-is instead of writing U+FFFD.
			first, upperNext = false, false
			b.WriteString(raw)
			continue
		}
		switch {
		case first:
			r = unicode.ToLower(r)
		case upperNext:
			r = unicode.ToUpper(r)
		case lowerRest:
			r = unicode.ToLower(r)
		}
		first, upperNext = false, false
		b.WriteRune(r)
	}

	return b.String()
}

func asciiToLower(c byte) byte {
	if 'A' <= c && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}

func asciiToUpper(c byte) byte {
	if 'a' <= c && c <= 'z' {
		return c - ('a' - 'A')
	}
	return c
}
