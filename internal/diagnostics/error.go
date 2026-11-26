// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package diagnostics

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"unicode"
	"unicode/utf8"

	corestrings "github.com/altessa-s/go-atlas-core/text/strings"
)

// DefaultMaxErrorLength limits error messages in completion records and span events.
const DefaultMaxErrorLength = 1024

const (
	redactedPlaceholder = "[REDACTED]"
	passwordKey         = "password"
	pwdKey              = "pwd"
)

// ErrorText removes URL user information, scheme-less DSN credentials, and
// password fragments before truncating the message, so even a password longer
// than the limit cannot leave a partial credential.
func ErrorText(s string, limit int) string {
	return corestrings.TruncateBytes(RedactURLUserinfo(stripControls(s)), limit, "...")
}

// RedactURLUserinfo removes credentials from URLs, scheme-less user:pass@
// tokens, and password=/pwd= fragments while preserving surrounding context.
func RedactURLUserinfo(s string) string {
	s = redactPasswordFragments(s)
	return redactTokenUserinfo(s)
}

func redactPasswordFragments(s string) string {
	if !containsPasswordKey(s) {
		return s
	}
	var out strings.Builder
	out.Grow(len(s))
	last := 0
	for i := 0; i < len(s); {
		keyLen := matchPasswordKey(s, i)
		if keyLen == 0 {
			i++
			continue
		}
		keyEnd := i + keyLen
		j := skipHorizontalSpace(s, keyEnd)
		if j >= len(s) || s[j] != '=' {
			i = keyEnd
			continue
		}
		valStart := skipHorizontalSpace(s, j+1)
		valEnd := scanPasswordValueEnd(s, valStart)

		tokStart := tokenStartBefore(s, i)
		if schemeIdx := strings.Index(s[tokStart:i], "://"); schemeIdx >= 0 {
			if beforeAt, _, hasAt := strings.CutLast(s[valStart:valEnd], "@"); hasAt {
				afterScheme := tokStart + schemeIdx + len("://")
				if !strings.ContainsAny(s[afterScheme:i], "/?#") {
					out.WriteString(s[last:afterScheme])
					last = valStart + len(beforeAt) + 1
					i = last
					continue
				}
			}
		}

		out.WriteString(s[last:keyEnd])
		out.WriteByte('=')
		out.WriteString(redactedPlaceholder)
		last = valEnd
		i = valEnd
	}
	if last == 0 {
		return s
	}
	out.WriteString(s[last:])
	return out.String()
}

func containsPasswordKey(s string) bool {
	for i := 0; i < len(s); i++ {
		if matchPasswordKey(s, i) > 0 {
			return true
		}
	}
	return false
}

func matchPasswordKey(s string, i int) int {
	if i > 0 && isASCIILetterOrDigit(s[i-1]) {
		return 0
	}
	if n := len(passwordKey); i+n <= len(s) && strings.EqualFold(s[i:i+n], passwordKey) && (i+n == len(s) || !isASCIILetterOrDigit(s[i+n])) {
		return n
	}
	if n := len(pwdKey); i+n <= len(s) && strings.EqualFold(s[i:i+n], pwdKey) && (i+n == len(s) || !isASCIILetterOrDigit(s[i+n])) {
		return n
	}
	return 0
}

func isASCIILetterOrDigit(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

func skipHorizontalSpace(s string, i int) int {
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return i
}

func scanPasswordValueEnd(s string, start int) int {
	if start >= len(s) {
		return start
	}
	if s[start] == '\'' || s[start] == '"' {
		quote := s[start]
		i := start + 1
		for i < len(s) {
			if s[i] == '\\' && i+1 < len(s) {
				i += 2
				continue
			}
			if s[i] == quote {
				if i+1 < len(s) && s[i+1] == quote {
					i += 2
					continue
				}
				return i + 1
			}
			i++
		}
		return len(s)
	}
	i := start
	for i < len(s) && !isUnquotedValueDelimiter(s[i]) {
		i++
	}
	return i
}

func isUnquotedValueDelimiter(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '\v', '\f', '&', ';', '"', '\'', ')', ']', '}', ',':
		return true
	default:
		return false
	}
}

func tokenStartBefore(s string, pos int) int {
	for i := pos; i > 0; {
		r, width := utf8.DecodeLastRuneInString(s[:i])
		if unicode.IsSpace(r) {
			return i
		}
		i -= width
	}
	return 0
}

func redactTokenUserinfo(s string) string {
	if !strings.Contains(s, "@") {
		return s
	}
	var out strings.Builder
	out.Grow(len(s))
	for i := 0; i < len(s); {
		r, width := utf8.DecodeRuneInString(s[i:])
		if unicode.IsSpace(r) {
			out.WriteString(s[i : i+width])
			i += width
			continue
		}
		end := i + width
		for end < len(s) {
			nr, nw := utf8.DecodeRuneInString(s[end:])
			if unicode.IsSpace(nr) {
				break
			}
			end += nw
		}
		out.WriteString(redactSingleToken(s[i:end]))
		i = end
	}
	return out.String()
}

func redactSingleToken(tok string) string {
	if !strings.Contains(tok, "@") {
		return tok
	}
	if strings.Contains(tok, "://") {
		return redactSchemeToken(tok)
	}
	return redactSchemelessToken(tok)
}

func redactSchemeToken(tok string) string {
	var out strings.Builder
	out.Grow(len(tok))
	pos := 0
	for pos < len(tok) {
		rel := strings.Index(tok[pos:], "://")
		if rel < 0 {
			out.WriteString(tok[pos:])
			break
		}
		afterScheme := pos + rel + len("://")
		out.WriteString(tok[pos:afterScheme])

		spanEnd := len(tok)
		if nextRel := strings.Index(tok[afterScheme:], "://"); nextRel >= 0 {
			// Split before the delimiter preceding the next scheme (e.g. comma-joined URLs).
			nextSchemePos := afterScheme + nextRel
			if delim := strings.LastIndexAny(tok[afterScheme:nextSchemePos], ",;|"); delim >= 0 {
				spanEnd = afterScheme + delim
			}
		}

		sub := tok[afterScheme:spanEnd]
		if _, host, ok := strings.CutLast(sub, "@"); ok {
			out.WriteString(host)
		} else {
			out.WriteString(sub)
		}
		pos = spanEnd
	}
	return out.String()
}

func redactSchemelessToken(tok string) string {
	before, host, ok := strings.CutLast(tok, "@")
	if !ok || before == "" || host == "" {
		return tok
	}
	userPrefix, _, ok := strings.Cut(before, ":")
	if !ok {
		return tok
	}
	start := 0
	if delim := strings.LastIndexAny(userPrefix, "\"'([{<,;="); delim >= 0 {
		start = delim + 1
	}
	return tok[:start] + host
}

// SanitizeText bounds a diagnostic field and removes control characters.
func SanitizeText(s string, limit int) string {
	return stripControls(corestrings.TruncateBytes(s, limit, "..."))
}

func stripControls(s string) string {
	s = strings.ToValidUTF8(s, "")
	if !strings.ContainsFunc(s, unicode.IsControl) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

// LogError formats a sanitized diagnostic only when the logger emits the record.
func LogError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := err.(diagnosticError); ok {
		return err
	}
	return diagnosticError{err}
}

type diagnosticError struct{ cause error }

func (e diagnosticError) Error() string        { return ErrorText(e.cause.Error(), DefaultMaxErrorLength) }
func (e diagnosticError) LogValue() slog.Value { return slog.StringValue(e.Error()) }

// ErrorClass classifies failures from injected components without rendering their text.
func ErrorClass(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	case errors.Is(err, io.ErrClosedPipe):
		return "closed_pipe"
	default:
		return "failed"
	}
}

// ClassifiedError exposes only a caller-supplied safe classification. Is can
// still match the callback's sentinels, while Unwrap cannot expose its text.
func ClassifiedError(class string, cause error) error {
	return &classifiedError{class: class, matches: func(target error) bool { return errors.Is(cause, target) }}
}

type classifiedError struct {
	class   string
	matches func(error) bool
}

func (e classifiedError) Error() string        { return e.class }
func (e classifiedError) Is(target error) bool { return e.matches(target) }
