// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"regexp"
	"strings"
)

// Policy tells a transport how one request treats idempotency keys. The
// zero value is [PolicyDefault].
type Policy int

const (
	// PolicyDefault applies the transport's configured mode: the key is
	// checked when present and required when enforceMandatory is set.
	PolicyDefault Policy = iota
	// PolicySkip ignores idempotency for the request, as an ignore list does.
	PolicySkip
	// PolicyOptional checks a key when present and never requires one.
	PolicyOptional
	// PolicyMandatory requires a key.
	PolicyMandatory
)

// Mandatory reports whether p requires a key, given the transport's
// enforceMandatory setting for [PolicyDefault].
func (p Policy) Mandatory(enforceMandatory bool) bool {
	return p == PolicyMandatory || (p == PolicyDefault && enforceMandatory)
}

// DefaultMaxKeyLength bounds the keys [OpaqueKeyValidator] and
// [PatternKeyValidator] accept, in bytes.
const DefaultMaxKeyLength = 128

// OpaqueKeyValidator accepts any key of 1 to maxLen bytes made of visible
// ASCII characters (0x21–0x7E: no space, control or non-ASCII byte), so a
// client may send its own request IDs instead of UUIDs. A maxLen below 1
// is [DefaultMaxKeyLength].
func OpaqueKeyValidator(maxLen int) func(string) error {
	if maxLen < 1 {
		maxLen = DefaultMaxKeyLength
	}
	return func(key string) error {
		if key == "" || len(key) > maxLen {
			return ErrInvalidFormat
		}
		for i := range len(key) {
			if c := key[i]; c < 0x21 || c > 0x7E {
				return ErrInvalidFormat
			}
		}
		return nil
	}
}

// PatternKeyValidator accepts a key of 1 to maxLen bytes that pattern matches
// in full (the match must span the whole key, whatever anchors pattern
// carries). A maxLen below 1 is [DefaultMaxKeyLength].
func PatternKeyValidator(pattern *regexp.Regexp, maxLen int) func(string) error {
	if maxLen < 1 {
		maxLen = DefaultMaxKeyLength
	}
	// Anchoring the whole pattern makes the match span the key even for an
	// alternation whose leftmost-first match would stop short. A valid
	// pattern stays valid inside a group.
	full := regexp.MustCompile(`\A(?:` + pattern.String() + `)\z`)
	return func(key string) error {
		if key == "" || len(key) > maxLen || !full.MatchString(key) {
			return ErrInvalidFormat
		}
		return nil
	}
}

// DefaultSkipVerbs are the method-name prefixes [VerbMethodPolicy] skips by
// default: Get, List and Search read, Update and Delete are guarded by etags
// or idempotent by definition, and so are their Batch forms. Create,
// BatchCreate and every other (custom) verb change state on each call.
var DefaultSkipVerbs = []string{"Get", "List", "Search", "Update", "Delete", "BatchGet", "BatchUpdate", "BatchDelete"}

// VerbMethodPolicy classifies a gRPC method by the verb its name starts
// with: a method whose name begins with one of skipVerbs as a whole word
// ("GetOrder", not "Getaway") is skipped, every other method gets p. Empty
// verbs are ignored: they would otherwise prefix every name and skip every
// method.
// fullMethod is "/package.Service/Method"; a method without a service part is
// classified by the whole name.
func VerbMethodPolicy(fullMethod string, p Policy, skipVerbs []string) Policy {
	name := fullMethod[strings.LastIndexByte(fullMethod, '/')+1:]
	for _, verb := range skipVerbs {
		if verb == "" {
			continue
		}
		if rest, ok := strings.CutPrefix(name, verb); ok && (rest == "" || isUpper(rest[0])) {
			return PolicySkip
		}
	}
	return p
}

// isUpper reports whether c is an ASCII upper-case letter: the start of the
// next word of a method name.
func isUpper(c byte) bool { return 'A' <= c && c <= 'Z' }
