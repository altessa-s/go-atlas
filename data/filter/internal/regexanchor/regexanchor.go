// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package regexanchor

import (
	"regexp/syntax"
	"strings"

	"github.com/altessa-s/go-atlas/data/filter"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// EndOfText returns pattern, a valid RE2 pattern, with every `$` that RE2
// reads as end of text rewritten to `\z`. A pattern without `$` is returned
// as is.
//
// The rewrite is checked against RE2's own parse: both patterns must parse
// to the same regular expression. A pattern that does not parse, or whose
// rewrite would change its meaning, fails with [filter.ErrInvalidRegex]
// rather than reaching the server in a form that matches something else.
func EndOfText(pattern string) (string, error) {
	if !strings.Contains(pattern, "$") {
		return pattern, nil
	}
	out := rewrite(pattern)
	if err := verify(pattern, out); err != nil {
		return "", err
	}
	return out, nil
}

// rewrite scans pattern as RE2 syntax and replaces each `$` outside
// multi-line mode with `\z`. It tracks escapes, `\Q…\E` quotes, character
// classes, groups and the scope of the `m` flag; everything else is copied
// byte for byte (no special character is a UTF-8 continuation byte, so a
// multi-byte rune passes through untouched).
func rewrite(pattern string) string {
	var b strings.Builder
	b.Grow(len(pattern) + strings.Count(pattern, "$"))

	multiLine := false
	var scopes []bool // the multi-line state to restore at each `)`

	for i := 0; i < len(pattern); i++ {
		switch c := pattern[i]; c {
		case '\\':
			if i+1 < len(pattern) && pattern[i+1] == 'Q' {
				end := strings.Index(pattern[i+2:], `\E`)
				if end < 0 {
					b.WriteString(pattern[i:])
					return b.String()
				}
				next := i + 2 + end + len(`\E`)
				b.WriteString(pattern[i:next])
				i = next - 1
				continue
			}
			end := min(i+escapeLen, len(pattern))
			b.WriteString(pattern[i:end])
			i = end - 1
		case '[':
			end := classEnd(pattern, i)
			b.WriteString(pattern[i:end])
			i = end - 1
		case '(':
			end, inner, scoped := groupStart(pattern, i, multiLine)
			b.WriteString(pattern[i:end])
			i = end - 1
			if scoped {
				scopes = append(scopes, multiLine)
			}
			multiLine = inner
		case ')':
			b.WriteByte(c)
			if n := len(scopes); n > 0 {
				multiLine, scopes = scopes[n-1], scopes[:n-1]
			}
		case '$':
			if multiLine {
				b.WriteByte(c)
			} else {
				b.WriteString(`\z`)
			}
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// escapeLen is the length of a single-byte escape: the backslash and the
// byte it escapes.
const escapeLen = len(`\x`)

// classEnd returns the index just past the character class opening at
// start. A `]` right after `[` or `[^` is a literal, an escape consumes the
// next byte, and a `[:name:]` class is skipped whole.
func classEnd(pattern string, start int) int {
	i := start + 1
	if i < len(pattern) && pattern[i] == '^' {
		i++
	}
	if i < len(pattern) && pattern[i] == ']' {
		i++
	}
	for i < len(pattern) {
		switch {
		case pattern[i] == '\\':
			i += escapeLen
		case strings.HasPrefix(pattern[i:], "[:"):
			if end := strings.Index(pattern[i+len("[:"):], ":]"); end >= 0 {
				i += len("[:") + end + len(":]")
			} else {
				i++
			}
		case pattern[i] == ']':
			return i + 1
		default:
			i++
		}
	}
	return len(pattern)
}

// groupStart reads the `(` at start and what introduces the group. It
// returns the index just past the introducer, the multi-line state that
// applies after it, and whether a new scope opened — false for a bare flag
// setting such as `(?m)`, which changes the enclosing scope and closes no
// group.
func groupStart(pattern string, start int, multiLine bool) (end int, inner, scoped bool) {
	i := start + 1
	if i >= len(pattern) || pattern[i] != '?' {
		return i, multiLine, true
	}
	i++
	if strings.HasPrefix(pattern[i:], "P<") || strings.HasPrefix(pattern[i:], "<") {
		return i, multiLine, true
	}
	set := true
	for ; i < len(pattern); i++ {
		switch pattern[i] {
		case '-':
			set = false
		case 'm':
			multiLine = set
		case ':':
			return i + 1, multiLine, true
		case ')':
			return i + 1, multiLine, false
		}
	}
	return i, multiLine, false
}

// verify reports whether out means exactly what pattern means to RE2. The
// rewritten `\z` parses like the original `$` except for the WasDollar
// marker, which is cleared before the two are compared.
func verify(pattern, out string) error {
	orig, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return coreerrs.Wrapf(filter.ErrInvalidRegex, "%v", err)
	}
	re, err := syntax.Parse(out, syntax.Perl)
	if err != nil {
		return coreerrs.Wrapf(filter.ErrInvalidRegex, "anchored pattern %q: %v", out, err)
	}
	clearWasDollar(orig)
	if orig.String() != re.String() {
		return coreerrs.Wrapf(filter.ErrInvalidRegex, "cannot anchor %q to the end of text", pattern)
	}
	return nil
}

// clearWasDollar drops the marker RE2 keeps on an end-of-text node spelled
// `$`, so that it prints as `\z`.
func clearWasDollar(re *syntax.Regexp) {
	if re.Op == syntax.OpEndText {
		re.Flags &^= syntax.WasDollar
	}
	for _, sub := range re.Sub {
		clearWasDollar(sub)
	}
}
