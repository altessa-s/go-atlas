// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package etag

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidTag is returned by [Parse] and [ParseList] when the input is not a
// syntactically valid entity-tag. Test for it with [errors.Is].
var ErrInvalidTag = errors.New("invalid entity-tag")

// Parse parses a single HTTP entity-tag such as `"abc"` or `W/"abc"` into a
// [Tag]. Surrounding whitespace is tolerated; any trailing content makes the
// input invalid. It returns an error wrapping [ErrInvalidTag] on malformed
// input.
func Parse(s string) (Tag, error) {
	t, rest, ok := parseOne(strings.TrimSpace(s))
	if !ok || strings.TrimSpace(rest) != "" {
		return Tag{}, fmt.Errorf("etag: parse %q: %w", s, ErrInvalidTag)
	}
	return t, nil
}

// ParseList parses a comma-separated list of entity-tags as found in an
// If-None-Match or If-Match header. The single token "*" yields star=true and
// an empty tags slice. An empty or malformed list returns an error wrapping
// [ErrInvalidTag].
func ParseList(s string) (tags []Tag, star bool, err error) {
	rest := strings.TrimSpace(s)
	if rest == "*" {
		return nil, true, nil
	}
	for {
		rest = strings.TrimLeft(rest, " \t")
		if rest == "" {
			break
		}
		t, after, ok := parseOne(rest)
		if !ok {
			return nil, false, fmt.Errorf("etag: parse list %q: %w", s, ErrInvalidTag)
		}
		tags = append(tags, t)
		rest = strings.TrimLeft(after, " \t")
		if rest == "" {
			break
		}
		if rest[0] != ',' {
			return nil, false, fmt.Errorf("etag: parse list %q: %w", s, ErrInvalidTag)
		}
		rest = rest[1:]
	}
	if len(tags) == 0 {
		return nil, false, fmt.Errorf("etag: parse list %q: %w", s, ErrInvalidTag)
	}
	return tags, false, nil
}

// parseOne consumes one entity-tag from the front of s and returns the parsed
// Tag, the unconsumed remainder, and whether parsing succeeded. The weak
// indicator "W/" is matched case-sensitively per RFC 7232.
func parseOne(s string) (Tag, string, bool) {
	weak := false
	if rest, ok := strings.CutPrefix(s, "W/"); ok {
		weak = true
		s = rest
	}
	if len(s) < 2 || s[0] != '"' {
		return Tag{}, s, false
	}
	for i := 1; i < len(s); i++ {
		switch c := s[i]; {
		case c == '"':
			return Tag{value: s[1:i], weak: weak}, s[i+1:], true
		case !isETagChar(c):
			return Tag{}, s, false
		}
	}
	return Tag{}, s, false // no closing quote
}

// isETagChar reports whether c is a legal etagc byte:
// etagc = %x21 / %x23-7E / obs-text (%x80-FF). The double quote (%x22) is
// excluded because it delimits the opaque-tag.
func isETagChar(c byte) bool {
	return c == 0x21 || (c >= 0x23 && c <= 0x7E) || c >= 0x80
}
