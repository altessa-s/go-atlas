// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package diagnostics

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/url"
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
	s = redactTokenUserinfo(s, true)
	s = redactPasswordFragments(s)
	return redactTokenUserinfo(s, false)
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
		valEnd := scanPasswordValueEnd(s, valStart, passwordSeparator(s, i))

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

// PostgreSQL keyword values are whitespace-delimited. Query parameters and
// semicolon-delimited connection strings use their own separators instead.
func passwordSeparator(s string, keyStart int) string {
	for keyStart > 0 && (s[keyStart-1] == ' ' || s[keyStart-1] == '\t') {
		keyStart--
	}
	if keyStart > 0 {
		switch s[keyStart-1] {
		case '?', '&':
			return "&#"
		case ';':
			return ";"
		}
	}
	return ""
}

func scanPasswordValueEnd(s string, start int, separators string) int {
	if start >= len(s) {
		return start
	}
	i := start
	if quote := s[i]; quote == '\'' || quote == '"' {
		i++
		for i < len(s) {
			if s[i] == '\\' && i+1 < len(s) {
				i += 2
				continue
			}
			if s[i] == quote {
				i++
				if i == len(s) || s[i] != quote {
					break
				}
			}
			i++
		}
	}
	for i < len(s) {
		if s[i] == '\\' && i+1 < len(s) {
			i += 2
			continue
		}
		r, width := utf8.DecodeRuneInString(s[i:])
		if unicode.IsSpace(r) || strings.ContainsRune(separators, r) {
			break
		}
		i += width
	}
	return i
}

func redactTokenUserinfo(s string, schemes bool) string {
	if !strings.Contains(s, "@") || (schemes && !strings.Contains(s, "://")) {
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
		out.WriteString(redactSingleToken(s[i:end], schemes))
		i = end
	}
	return out.String()
}

func redactSingleToken(tok string, schemes bool) string {
	if !strings.Contains(tok, "@") {
		return tok
	}
	if strings.Contains(tok, "://") {
		if schemes {
			return redactSchemeToken(tok)
		}
	} else if !schemes {
		return redactSchemelessToken(tok)
	}
	return tok
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
		out.WriteString(sub[urlUserinfoEnd(sub):])
		pos = spanEnd
	}
	return out.String()
}

func urlUserinfoEnd(sub string) int {
	at := strings.LastIndexByte(sub, '@')
	if at < 0 {
		return 0
	}
	query := strings.IndexByte(sub, '?')
	if query < 0 || at < query || strings.Contains(sub[:query], "#") {
		return at + 1
	}
	// A valid URL authority makes an @ in the query ordinary data. Keep
	// looking through the pre-query text for malformed userinfo containing a
	// slash, which can also look like a valid host and path.
	authority, _, _ := strings.Cut(sub[:query], "/")
	if u, err := url.Parse("http://" + authority); err == nil && u.Host != "" {
		return strings.LastIndexByte(sub[:query], '@') + 1
	}
	return at + 1
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
