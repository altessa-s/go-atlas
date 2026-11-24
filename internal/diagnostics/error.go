// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package diagnostics sanitizes error text for logs, spans and persisted outcomes.
package diagnostics

import (
	"log/slog"
	"regexp"
	"strings"
	"unicode"

	corestrings "github.com/altessa-s/go-atlas-core/text/strings"
)

// DefaultMaxErrorLength limits error messages in completion records and span events.
const DefaultMaxErrorLength = 1024

var urlUserinfo = regexp.MustCompile(`(://)[^/?#\s]*@`)

// ErrorText removes URL user information before truncating the message, so
// even a password longer than the limit cannot leave a partial credential.
func ErrorText(s string, limit int) string {
	return corestrings.TruncateBytes(RedactURLUserinfo(stripControls(s)), limit, "...")
}

// RedactURLUserinfo removes credentials from URLs while preserving the surrounding bytes.
func RedactURLUserinfo(s string) string {
	if strings.Contains(s, "://") && strings.Contains(s, "@") {
		return urlUserinfo.ReplaceAllString(s, "${1}")
	}
	return s
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
	return diagnosticError{err}
}

type diagnosticError struct{ cause error }

func (e diagnosticError) Error() string        { return ErrorText(e.cause.Error(), DefaultMaxErrorLength) }
func (e diagnosticError) LogValue() slog.Value { return slog.StringValue(e.Error()) }
