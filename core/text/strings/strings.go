// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package strings

import (
	"crypto/subtle"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
	"unsafe"

	"github.com/altessa-s/go-atlas/core/types/constraints"
)

// caseInsensitiveRegexCache caches compiled case-insensitive regexes keyed by separator.
var caseInsensitiveRegexCache sync.Map // map[string]*regexp.Regexp

// IsEmpty reports whether str is empty or contains only whitespace characters.
// The generic type parameter accepts both string and *string values. A nil
// *string is treated as empty and returns true. Whitespace detection uses
// [strings.TrimSpace], so all Unicode whitespace code points are recognized.
//
// For a non-generic, allocation-free alternative that works only on plain
// strings, see [IsEmptyOrWhitespace].
//
// Example:
//
//	IsEmpty("")        // true
//	IsEmpty("  ")      // true
//	IsEmpty("hello")   // false
func IsEmpty[T ~string | ~*string](str T) bool {
	switch v := any(str).(type) {
	case string:
		return len(strings.TrimSpace(v)) == 0
	case *string:
		if v == nil {
			return true
		}
		return len(strings.TrimSpace(*v)) == 0
	default:
		// This should never happen with the type constraint
		// but we return true for safety (empty by default)
		return true
	}
}

// Pre-instantiated converters for common numeric types. Each variable is a
// specialization of the generic [To] function, avoiding the need for explicit
// type parameters at call sites.
//
// Example:
//
//	n := ToInt64("42")      // int64(42)
//	f := ToFloat64("3.14")  // float64(3.14)
var (
	ToUint    = To[uint]
	ToUint64  = To[uint64]
	ToUint32  = To[uint32]
	ToUint16  = To[uint16]
	ToUint8   = To[uint8]
	ToInt     = To[int]
	ToInt64   = To[int64]
	ToInt32   = To[int32]
	ToInt16   = To[int16]
	ToInt8    = To[int8]
	ToFloat32 = To[float32]
	ToFloat64 = To[float64]
)

// ToPtr converts a string or *string into a *string, returning nil when the
// value is empty or contains only whitespace. For *string inputs, the original
// pointer is returned if it passes the non-empty check; for string inputs a
// new pointer is allocated. Nil *string inputs return nil.
//
// Example:
//
//	ToPtr("hello")  // &"hello"
//	ToPtr("")       // nil
//	ToPtr("  ")     // nil
func ToPtr[T interface{ ~string | ~*string }](str T) *string {
	switch t := any(str).(type) {
	case *string:
		if t == nil || strings.TrimSpace(*t) == "" {
			return nil
		}
		return t
	case string:
		if t == "" || strings.TrimSpace(t) == "" {
			return nil
		}
		return &t
	}
	return nil
}

// FromPtr dereferences ptr and returns the pointed-to string. If ptr is nil,
// the empty string is returned. This is the inverse of [ToPtr].
//
// Example:
//
//	s := "hello"
//	FromPtr(&s)   // "hello"
//	FromPtr(nil)  // ""
func FromPtr(ptr *string) string {
	if ptr == nil {
		return ""
	}
	return *ptr
}

// To parses str into a numeric value of type T. Supported types are all signed
// and unsigned integer types, as well as float32 and float64 (constrained by
// [constraints.Numbers]). On a parse error the zero value of T is returned
// with no error indication; callers that need error details should use the
// [strconv] package directly.
//
// # Safety of unsafe.Sizeof
//
// The use of `unsafe.Sizeof(t)` is safe because:
//
//  1. Compile-Time Constant: For a given type T, Sizeof returns a compile-time constant.
//     It doesn't access any memory; it merely returns the size of the type in bytes.
//
//  2. No Pointer Dereference: Sizeof operates on the type, not on any value's memory.
//     Even though `t` is the zero value, Sizeof doesn't read from `t`'s address.
//
//  3. Standard Pattern: This is the idiomatic way to get a type's bit size for
//     strconv.Parse* functions in generic code.
//
// Example:
//
//	To[int]("42")       // 42
//	To[float64]("3.14") // 3.14
//	To[int]("invalid")  // 0
func To[T constraints.Numbers](str string) T {
	var t T
	typ := reflect.TypeFor[T]()
	kind := typ.Kind()
	const bitsPerByte = 8
	// Safe: Sizeof returns the size of type T, doesn't access memory
	bitSize := int(unsafe.Sizeof(t) * bitsPerByte)

	switch kind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		d, err := strconv.ParseInt(str, 10, bitSize)
		if err != nil {
			return t
		}
		return T(d)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		d, err := strconv.ParseUint(str, 10, bitSize)
		if err != nil {
			return t
		}
		return T(d)
	case reflect.Float32, reflect.Float64:
		d, err := strconv.ParseFloat(str, bitSize)
		if err != nil {
			return t
		}
		return T(d)
	default:
		// The type constraint ensures numeric kinds; keep a safe fallback.
		return t
	}
}

// IsEmptyOrWhitespace reports whether s is zero-length or consists entirely of
// Unicode whitespace code points. Unlike [IsEmpty], it operates only on plain
// strings (no pointer support) and avoids the allocation that
// [strings.TrimSpace] would cause by iterating runes directly.
//
// Example:
//
//	IsEmptyOrWhitespace("")      // true
//	IsEmptyOrWhitespace("  \t")  // true
//	IsEmptyOrWhitespace("a")     // false
func IsEmptyOrWhitespace(s string) bool {
	if len(s) == 0 {
		return true
	}

	for _, r := range s {
		if !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

// ToBytesUnsafe converts a string to []byte without copying the underlying data.
//
// WARNING: The returned slice must NEVER be modified. Modifying it causes undefined
// behavior because Go strings are immutable and may be stored in read-only memory.
//
// # Safety Invariants
//
// This function is safe when the caller adheres to the read-only contract:
//
//  1. Immutability Contract: The caller must treat the returned []byte as read-only.
//     Any modification (including via sub-slices) results in undefined behavior.
//
//  2. Go 1.20+ Safety: Uses unsafe.StringData and unsafe.Slice, which are the
//     officially supported functions for this conversion pattern since Go 1.20.
//     These functions are designed specifically for zero-copy string/byte conversions.
//
//  3. Lifetime: The returned slice is valid only as long as the source string is live.
//     If the string is garbage collected, the slice becomes a dangling pointer.
//     In practice, this is safe because the caller typically holds the string.
//
//  4. Empty String Safety: Returns nil for empty strings to avoid creating a slice
//     header pointing to potentially invalid memory.
//
// Use Cases:
//   - Passing string data to APIs that accept []byte for reading (e.g., hash functions)
//   - Avoiding allocation in hot paths where the data is only read
//
// Example:
//
//	b := ToBytesUnsafe("hello")  // []byte{'h','e','l','l','o'}
//
// #nosec G103 -- intentional zero-copy conversion with documented safety contract
func ToBytesUnsafe(str string) []byte {
	if len(str) == 0 {
		return nil
	}
	return unsafe.Slice(unsafe.StringData(str), len(str))
}

// FromBytesUnsafe converts []byte to string without copying the underlying data.
//
// WARNING: The source byte slice must NOT be modified after this conversion.
// Modifying it would change the "immutable" string, violating Go's string semantics
// and potentially causing data corruption in maps, comparisons, etc.
//
// # Safety Invariants
//
// This function is safe when the caller adheres to the immutability contract:
//
//  1. Post-Conversion Immutability: The source []byte must not be modified after
//     calling this function. The string and slice share the same backing array.
//
//  2. Go 1.20+ Safety: Uses unsafe.SliceData and unsafe.String, which are the
//     officially supported functions for this conversion pattern since Go 1.20.
//
//  3. Common Safe Patterns:
//     - Slice is locally created and immediately converted (never modified again)
//     - Slice comes from a read-only source (e.g., mmap'd file)
//     - Slice ownership is transferred to the string (caller discards slice reference)
//
//  4. Empty Slice Safety: Returns empty string "" for nil or zero-length slices
//     to avoid creating a string header pointing to potentially invalid memory.
//
// Use Cases:
//   - Converting buffers that won't be reused (e.g., one-shot parsing)
//   - Avoiding allocation when returning string from []byte computation
//
// Example:
//
//	s := FromBytesUnsafe([]byte("hello"))  // "hello"
//
// #nosec G103 -- intentional zero-copy conversion with documented safety contract
func FromBytesUnsafe(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(unsafe.SliceData(b), len(b))
}

// JoinOptions configures the behavior of the [Join] function, controlling the
// separator placed between elements, optional prefix/suffix wrapping, and
// whether empty strings are excluded from the output.
type JoinOptions struct {
	Separator string // Separator is the string to place between elements.
	SkipEmpty bool   // SkipEmpty when true, excludes empty strings from joining.
	Prefix    string // Prefix is prepended to the result if it's not empty.
	Suffix    string // Suffix is appended to the result if it's not empty.
}

// Join concatenates elements into a single string using the rules specified in
// opts. When [JoinOptions.SkipEmpty] is true, empty strings are removed before
// joining. If the joined result is non-empty and [JoinOptions.Prefix] or
// [JoinOptions.Suffix] are set, they are prepended and appended respectively.
// Returns the empty string when elements is empty or all elements are empty
// with SkipEmpty enabled.
//
// Example:
//
//	parts := []string{"a", "", "b"}
//	Join(parts, JoinOptions{Separator: ","})                  // "a,,b"
//	Join(parts, JoinOptions{Separator: ",", SkipEmpty: true}) // "a,b"
func Join(elements []string, opts JoinOptions) string {
	if len(elements) == 0 {
		return ""
	}

	var filteredElements []string
	if opts.SkipEmpty {
		for _, elem := range elements {
			if elem != "" {
				filteredElements = append(filteredElements, elem)
			}
		}
	} else {
		filteredElements = elements
	}

	if len(filteredElements) == 0 {
		return ""
	}

	result := strings.Join(filteredElements, opts.Separator)
	if result != "" {
		result = opts.Prefix + result + opts.Suffix
	}

	return result
}

// SplitOptions configures the behavior of [Split] and [SplitSeq], controlling
// the separator, maximum number of splits, whitespace trimming, empty-element
// removal, and case sensitivity.
type SplitOptions struct {
	Separator     string // Separator is the string to split on.
	MaxSplits     int    // MaxSplits limits the number of splits. -1 means no limit.
	TrimSpace     bool   // TrimSpace when true, trims whitespace from each resulting part.
	SkipEmpty     bool   // SkipEmpty when true, excludes empty strings from the result.
	CaseSensitive bool   // CaseSensitive when false, performs case-insensitive splitting.
}

// Split divides s into substrings around occurrences of
// [SplitOptions.Separator] and returns the resulting slice. The behavior is
// further controlled by [SplitOptions]: case-insensitive matching, a maximum
// number of splits, per-element whitespace trimming, and removal of empty
// elements.
//
// When [SplitOptions.CaseSensitive] is false and the lowercased separator has
// a different byte length than the original (due to Unicode special casing), a
// compiled regular expression is used for correctness and cached for
// subsequent calls.
//
// Returns nil when s is empty and [SplitOptions.SkipEmpty] is true. For a
// lazy, allocation-free alternative see [SplitSeq].
//
// Example:
//
//	Split("a,b,c", SplitOptions{Separator: ","})                      // ["a", "b", "c"]
//	Split("a,,b", SplitOptions{Separator: ",", SkipEmpty: true})      // ["a", "b"]
func Split(s string, opts SplitOptions) []string {
	if s == "" {
		if opts.SkipEmpty {
			return nil
		}
		return []string{""}
	}

	var parts []string
	separator := opts.Separator

	// Handle case-insensitive splitting
	if !opts.CaseSensitive && separator != "" {
		lowerS := strings.ToLower(s)
		lowerSep := strings.ToLower(separator)

		// Fast path: if lengths match, we can use simple indexing map
		if len(lowerS) == len(s) && len(lowerSep) == len(separator) {
			var start int
			for {
				if opts.MaxSplits > 0 && len(parts) >= opts.MaxSplits {
					parts = append(parts, s[start:])
					break
				}

				idx := strings.Index(lowerS[start:], lowerSep)
				if idx == -1 {
					parts = append(parts, s[start:])
					break
				}

				actualIdx := start + idx
				parts = append(parts, s[start:actualIdx])
				start = actualIdx + len(separator)
			}
		} else {
			// Slow path: lengths differ (special casing or invalid UTF-8).
			// Use cached regexp for correctness.
			var re *regexp.Regexp
			var err error
			if cached, ok := caseInsensitiveRegexCache.Load(separator); ok {
				re = cached.(*regexp.Regexp) //nolint:errcheck // sync.Map guarantees correct type
			} else {
				quote := regexp.QuoteMeta(separator)
				re, err = regexp.Compile("(?i)" + quote)
				if err == nil {
					caseInsensitiveRegexCache.Store(separator, re)
				}
			}
			if err != nil {
				// If regexp fails (e.g. invalid UTF-8 in separator), fall back to exact split
				// This is a reasonable fallback for garbage inputs
				if opts.MaxSplits <= 0 {
					parts = strings.Split(s, separator)
				} else {
					parts = strings.SplitN(s, separator, opts.MaxSplits+1)
				}
			} else {
				// Handle MaxSplits for regexp
				n := -1
				if opts.MaxSplits > 0 {
					n = opts.MaxSplits + 1
				}
				parts = re.Split(s, n)
			}
		}
	} else {
		// Case-sensitive splitting (standard behavior)
		if opts.MaxSplits <= 0 {
			parts = strings.Split(s, separator)
		} else {
			parts = strings.SplitN(s, separator, opts.MaxSplits+1)
		}
	}

	// Process the parts based on options
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if opts.TrimSpace {
			part = strings.TrimSpace(part)
		}
		if opts.SkipEmpty && part == "" {
			continue
		}
		result = append(result, part)
	}

	if len(result) == 0 && opts.SkipEmpty {
		return nil
	}

	return result
}

// ContainsOptions configures the behavior of the [Contains] function,
// controlling case sensitivity, whole-word matching, and occurrence counting.
type ContainsOptions struct {
	CaseSensitive   bool // CaseSensitive when false, performs case-insensitive search.
	MatchWholeWords bool // MatchWholeWords when true, only matches complete words.
	Count           bool // Count when true, returns the number of occurrences instead of just presence.
}

// ContainsResult holds the outcome of a [Contains] search, including whether
// the substring was found, how many times it occurred, and the byte positions
// of each match (populated only when [ContainsOptions.Count] is true).
type ContainsResult struct {
	Found     bool  // Found indicates whether the substring was found.
	Count     int   // Count is the number of occurrences (only set when ContainsOptions.Count is true).
	Positions []int // Positions contains the start positions of all matches (only when Count is true).
}

// Contains searches for substr within s according to opts and returns a
// [ContainsResult]. An empty substr always matches at position 0. When
// [ContainsOptions.CaseSensitive] is false the search is lowercased. When
// [ContainsOptions.MatchWholeWords] is true, matches that are adjacent to
// letters or digits are rejected. When [ContainsOptions.Count] is true, all
// non-overlapping occurrences are found and their positions recorded;
// otherwise the function returns as soon as the first match is confirmed.
//
// Example:
//
//	r := Contains("Hello World", "hello", ContainsOptions{CaseSensitive: false})
//	// r.Found = true
func Contains(s, substr string, opts ContainsOptions) ContainsResult {
	if substr == "" {
		return ContainsResult{Found: true, Count: 1, Positions: []int{0}}
	}

	searchStr := s
	searchSubstr := substr

	if !opts.CaseSensitive {
		searchStr = strings.ToLower(s)
		searchSubstr = strings.ToLower(substr)
	}

	if !opts.Count && !opts.MatchWholeWords {
		// Simple case: just check if it contains the substring
		found := strings.Contains(searchStr, searchSubstr)
		return ContainsResult{Found: found}
	}

	var positions []int
	var count int
	start := 0

	for {
		idx := strings.Index(searchStr[start:], searchSubstr)
		if idx == -1 {
			break
		}

		actualPos := start + idx

		if opts.MatchWholeWords {
			// Check if this is a whole word match
			if !isWholeWordMatch(s, actualPos, len(substr)) {
				start = actualPos + 1
				continue
			}
		}

		count++
		if opts.Count {
			positions = append(positions, actualPos)
		}

		if !opts.Count {
			// If we don't need count, we can return early
			return ContainsResult{Found: true}
		}

		start = actualPos + len(substr)
	}

	return ContainsResult{
		Found:     count > 0,
		Count:     count,
		Positions: positions,
	}
}

// isWholeWordMatch checks if a match at given position is a complete word.
func isWholeWordMatch(s string, pos, length int) bool {
	// Check character before the match
	if pos > 0 {
		prevChar := rune(s[pos-1])
		if unicode.IsLetter(prevChar) || unicode.IsDigit(prevChar) {
			return false
		}
	}

	// Check character after the match
	endPos := pos + length
	if endPos < len(s) {
		nextChar := rune(s[endPos])
		if unicode.IsLetter(nextChar) || unicode.IsDigit(nextChar) {
			return false
		}
	}

	return true
}

// SecureCompare performs a constant-time comparison of provided and expected
// using [crypto/subtle.ConstantTimeCompare], returning true only when they are
// byte-identical. Use this instead of == when comparing secrets such as
// passwords, API keys, or HMAC digests to prevent timing side-channel attacks.
//
// Note: both strings are converted to []byte, which allocates. For
// repeated comparisons of sensitive data, consider [SecureString.Equal].
//
// Example:
//
//	SecureCompare(userToken, expectedToken)  // true if equal
func SecureCompare(provided, expected string) bool {
	return subtle.ConstantTimeCompare(ToBytesUnsafe(provided), ToBytesUnsafe(expected)) == 1
}

// TimingSafePrefixMatch reports whether s starts with prefix using a
// constant-time, case-insensitive comparison via
// [crypto/subtle.ConstantTimeCompare]. If s is shorter than prefix, it is
// padded with null bytes so that the comparison length does not leak through
// timing.
//
// Use this when verifying security-relevant prefixes (e.g. authorization
// scheme headers) where a timing side-channel could reveal the expected value.
//
// Example:
//
//	TimingSafePrefixMatch("Bearer token", "bearer")  // true
func TimingSafePrefixMatch(s, prefix string) bool {
	if len(s) < len(prefix) {
		// Extend s to prefix length to avoid timing leaks
		s += strings.Repeat("\x00", len(prefix)-len(s))
	}

	// Convert both to lowercase for case-insensitive comparison
	sLower := strings.ToLower(s[:len(prefix)])
	prefixLower := strings.ToLower(prefix)

	// Use constant-time comparison
	return subtle.ConstantTimeCompare(ToBytesUnsafe(sLower), ToBytesUnsafe(prefixLower)) == 1
}

// SubstringMatch reports whether s contains substr using a case-insensitive
// comparison. If s is shorter than substr, s is padded with null bytes to
// avoid an early false return based on length alone.
//
// WARNING: This function is NOT constant-time. The standard
// [strings.Contains] call may reveal match position through timing. For
// security-sensitive searches, use [TimingSafeSubstringMatch] instead.
//
// Example:
//
//	SubstringMatch("Hello World", "world")  // true
func SubstringMatch(s, substr string) bool {
	if len(s) < len(substr) {
		// Extend s to substr length to avoid timing leaks
		s += strings.Repeat("\x00", len(substr)-len(s))
	}
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// TimingSafeSubstringMatch reports whether s contains substr using a
// constant-time, case-insensitive search. Every possible starting position
// in s is examined with [crypto/subtle.ConstantTimeCompare] so that neither
// the presence nor the position of the match is revealed through timing
// differences. An empty substr always returns true.
//
// The function lowercases both inputs, so it has O(len(s) * len(substr))
// comparison cost. Use this only when timing leaks are a genuine concern.
//
// Example:
//
//	TimingSafeSubstringMatch("Hello World", "world")  // true
func TimingSafeSubstringMatch(s, substr string) bool {
	if substr == "" {
		return true
	}

	sLower := strings.ToLower(s)
	subLower := ToBytesUnsafe(strings.ToLower(substr))
	n := len(subLower)

	if len(sLower) < n {
		return false
	}

	sLowerBytes := ToBytesUnsafe(sLower)
	found := 0
	for i := 0; i <= len(sLowerBytes)-n; i++ {
		found |= subtle.ConstantTimeCompare(sLowerBytes[i:i+n], subLower)
	}
	return found == 1
}

// StringEqualsUnsafe compares two strings for equality with a fast path that
// checks whether both share the same backing memory (common when strings have
// been interned). If the lengths differ, it falls back to
// [strings.EqualFold] for a case-insensitive comparison. For same-length
// strings with different data pointers, a standard == comparison is used.
//
// # Safety Invariants
//
// This function uses unsafe.StringData for pointer comparison optimization:
//
//  1. Read-Only Access: We only compare pointer values, never dereference them.
//     This is equivalent to comparing memory addresses.
//
//  2. Go 1.20+ Safety: unsafe.StringData is the officially supported way to get
//     a string's underlying data pointer since Go 1.20.
//
//  3. String Interning Optimization: Go may intern identical string literals,
//     causing them to share the same backing memory. This check detects that case
//     in O(1) time, avoiding character-by-character comparison.
//
//  4. Correctness: If pointers differ, we fall back to standard string comparison
//     which is always correct. The unsafe path is purely an optimization.
//
// Example:
//
//	StringEqualsUnsafe("hello", "hello")  // true
//
// #nosec G103 -- intentional pointer comparison for performance optimization
func StringEqualsUnsafe(a, b string) bool {
	if len(a) != len(b) {
		return strings.EqualFold(a, b) // Fallback to Unicode comparison
	}

	if len(a) == 0 {
		return true
	}

	// Get string data pointers using modern unsafe functions.
	// This is safe because we only compare addresses, never dereference.
	aPtr := unsafe.StringData(a)
	bPtr := unsafe.StringData(b)

	// If they point to the same memory, they're equal (string interning optimization)
	if aPtr == bPtr {
		return true
	}

	// Fall back to regular comparison
	return a == b
}

// Concat concatenates parts into a single string using a pooled
// [strings.Builder] from [StringBuilderPool]. The builder is pre-grown to the
// exact total length, so only one allocation occurs for the final string.
// Returns the empty string for zero parts and the original string for one part.
//
// Example:
//
//	Concat("hello", " ", "world")  // "hello world"
func Concat(parts ...string) string {
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return parts[0]
	}

	builder := GetStringBuilder()
	defer PutStringBuilder(builder)

	totalLen := 0
	for _, part := range parts {
		totalLen += len(part)
	}
	builder.Grow(totalLen)

	for _, part := range parts {
		builder.WriteString(part)
	}

	return builder.String()
}

// ConcatUnsafe concatenates parts into a single string with exactly one heap
// allocation for the combined byte buffer. The returned string is created via
// [FromBytesUnsafe], meaning it shares its backing array with the internal
// buffer. This is safe because the buffer is not retained or modified after
// the function returns.
//
// Returns the empty string for zero parts and the original string for one part.
//
// Example:
//
//	ConcatUnsafe("hello", " ", "world")  // "hello world"
func ConcatUnsafe(parts ...string) string {
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return parts[0]
	}

	// Calculate total length
	totalLen := 0
	for _, s := range parts {
		totalLen += len(s)
	}

	if totalLen == 0 {
		return ""
	}

	// Allocate once
	result := make([]byte, 0, totalLen)
	for _, s := range parts {
		result = append(result, s...)
	}

	return FromBytesUnsafe(result)
}

// IsLowercaseUnsafe reports whether every letter in s is lowercase, using a
// fast byte-level ASCII scan via [ToBytesUnsafe]. If any non-ASCII byte is
// encountered, it falls back to the allocation-free [IsLowercase] for correct
// Unicode handling. Empty strings return true.
//
// Example:
//
//	IsLowercaseUnsafe("hello")  // true
//	IsLowercaseUnsafe("Hello")  // false
func IsLowercaseUnsafe(s string) bool {
	return isCaseUnsafe(s, 'A', 'Z', IsLowercase)
}

// IsLowercase reports whether every letter in s (as determined by
// [unicode.IsLetter]) is lowercase. Non-letter runes are ignored. Empty
// strings return true.
//
// Example:
//
//	IsLowercase("hello123")  // true
//	IsLowercase("Hello")     // false
func IsLowercase(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) && !unicode.IsLower(r) {
			return false
		}
	}
	return true
}

// IsUppercaseUnsafe reports whether every letter in s is uppercase, using a
// fast byte-level ASCII scan via [ToBytesUnsafe]. If any non-ASCII byte is
// encountered, it falls back to the allocation-free [IsUppercase] for correct
// Unicode handling. Empty strings return true.
//
// Example:
//
//	IsUppercaseUnsafe("HELLO")  // true
//	IsUppercaseUnsafe("Hello")  // false
func IsUppercaseUnsafe(s string) bool {
	return isCaseUnsafe(s, 'a', 'z', IsUppercase)
}

func isCaseUnsafe(s string, asciiBadMin, asciiBadMax byte, fallback func(string) bool) bool {
	if len(s) == 0 {
		return true
	}

	// Fast path for ASCII-only strings
	const asciiThreshold = 0x80 // Characters >= 0x80 are non-ASCII
	bytes := ToBytesUnsafe(s)
	for _, b := range bytes {
		if b >= asciiThreshold {
			return fallback(s)
		}
		if b >= asciiBadMin && b <= asciiBadMax {
			return false
		}
	}
	return true
}

// IsUppercase reports whether every letter in s (as determined by
// [unicode.IsLetter]) is uppercase. Non-letter runes are ignored. Empty
// strings return true.
//
// Example:
//
//	IsUppercase("HELLO123")  // true
//	IsUppercase("Hello")     // false
func IsUppercase(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) && !unicode.IsUpper(r) {
			return false
		}
	}
	return true
}

// IsASCIISpace reports whether b is one of the six ASCII whitespace bytes:
// space (0x20), horizontal tab (0x09), newline (0x0A), vertical tab (0x0B),
// form feed (0x0C), or carriage return (0x0D). It does not recognize Unicode
// whitespace code points beyond ASCII.
//
// Example:
//
//	IsASCIISpace(' ')   // true
//	IsASCIISpace('\t')  // true
//	IsASCIISpace('a')   // false
func IsASCIISpace(b byte) bool {
	// Create a bitmask for ASCII space characters
	// ' '(32), '\t'(9), '\n'(10), '\r'(13), '\v'(11), '\f'(12)
	return b == 32 || (b >= 9 && b <= 13 && b != 11) || b == 11
}

// IsTrimmed reports whether s has no leading or trailing whitespace. For
// ASCII-only strings the check uses [IsASCIISpace]; for strings with non-ASCII
// first or last characters the rune is decoded with [utf8.DecodeRuneInString]
// and tested with [unicode.IsSpace]. Returns true for the empty string.
//
// Example:
//
//	IsTrimmed("hello")   // true
//	IsTrimmed(" hello")  // false
//	IsTrimmed("")        // true
func IsTrimmed(s string) bool {
	const MaxASCIICode = 128

	if len(s) == 0 {
		return true
	}

	// Check first and last characters for whitespace
	firstRune := rune(s[0])
	lastRune := rune(s[len(s)-1])

	// For ASCII characters, we can check directly
	if firstRune < MaxASCIICode && lastRune < MaxASCIICode {
		return !IsASCIISpace(byte(firstRune)) && !IsASCIISpace(byte(lastRune))
	}

	// For non-ASCII, use utf8 package to decode runes without allocation
	firstRune, _ = utf8.DecodeRuneInString(s)
	lastRune, _ = utf8.DecodeLastRuneInString(s)
	return !unicode.IsSpace(firstRune) && !unicode.IsSpace(lastRune)
}

// IsTrimmedUnsafe reports whether s has no leading or trailing whitespace,
// using [ToBytesUnsafe] for zero-copy byte-level access. For ASCII bytes the
// check uses [IsASCIISpace]; for non-ASCII bytes the first/last rune is
// decoded with [utf8.DecodeRune] and tested with [unicode.IsSpace]. Returns
// true for the empty string.
//
// WARNING: Assumes the input is valid UTF-8 for non-ASCII characters. Invalid
// sequences may produce incorrect results.
//
// Example:
//
//	IsTrimmedUnsafe("hello")  // true
//	IsTrimmedUnsafe(" hi ")   // false
func IsTrimmedUnsafe(s string) bool {
	if len(s) == 0 {
		return true
	}

	// Get direct access to string bytes
	bytes := ToBytesUnsafe(s)

	// Check the first byte for ASCII whitespace
	const asciiThreshold = 0x80 // Characters >= 0x80 are non-ASCII
	firstByte := bytes[0]
	if firstByte < asciiThreshold { // ASCII
		if IsASCIISpace(firstByte) {
			return false
		}
	} else {
		// Non-ASCII: decode first rune manually using unsafe access
		firstRune, _ := utf8.DecodeRune(bytes)
		if unicode.IsSpace(firstRune) {
			return false
		}
	}

	// Check last byte for ASCII whitespace
	lastByte := bytes[len(bytes)-1]
	if lastByte < asciiThreshold { // ASCII
		return !IsASCIISpace(lastByte)
	}

	// Non-ASCII: decode last rune manually using unsafe access
	lastRune, _ := utf8.DecodeLastRune(bytes)
	return !unicode.IsSpace(lastRune)
}

// TrimSuffixFast removes suffix from the end of s if present, using direct
// substring comparison rather than [strings.TrimSuffix]. Returns s unchanged
// when suffix is empty, s is shorter than suffix, or suffix is not found.
// Because Go strings are backed by shared memory, the returned substring never
// allocates.
//
// Example:
//
//	TrimSuffixFast("hello/", "/")    // "hello"
//	TrimSuffixFast("hello", "/")     // "hello" (no allocation)
//	TrimSuffixFast("test.go", ".go") // "test"
func TrimSuffixFast(s, suffix string) string {
	if len(suffix) == 0 || len(s) < len(suffix) {
		return s
	}
	if s[len(s)-len(suffix):] == suffix {
		return s[:len(s)-len(suffix)]
	}
	return s
}

// TrimPrefixFast removes prefix from the beginning of s if present, using
// direct substring comparison rather than [strings.TrimPrefix]. Returns s
// unchanged when prefix is empty, s is shorter than prefix, or prefix is not
// found. Because Go strings are backed by shared memory, the returned
// substring never allocates.
//
// Example:
//
//	TrimPrefixFast("/hello", "/")     // "hello"
//	TrimPrefixFast("hello", "/")      // "hello" (no allocation)
//	TrimPrefixFast("test.go", "test") // ".go"
func TrimPrefixFast(s, prefix string) string {
	if len(prefix) == 0 || len(s) < len(prefix) {
		return s
	}
	if s[:len(prefix)] == prefix {
		return s[len(prefix):]
	}
	return s
}
