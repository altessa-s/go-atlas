// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package diagnostics

import (
	"context"
	"errors"
	"io"
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
