// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit

import "strings"

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

// PolicyMode selects which decisions a [Recorder] records.
type PolicyMode int

const (
	// PolicyDenyOnly records only denials. It is the default: it captures the
	// security-relevant events while keeping volume bounded.
	PolicyDenyOnly PolicyMode = iota
	// PolicyAll records both grants and denials — a full access trail, at the
	// cost of one sink write per authorized request.
	PolicyAll
)

// String implements [fmt.Stringer].
func (m PolicyMode) String() string {
	switch m {
	case PolicyDenyOnly:
		return "deny_only"
	case PolicyAll:
		return "all"
	default:
		return "unknown"
	}
}

// FailureMode selects what a [Sink] write error means.
type FailureMode int

const (
	// FailureBestEffort discards sink errors: a failed audit write never blocks
	// the request. It is the default — auditing must not become a
	// denial-of-service vector.
	FailureBestEffort FailureMode = iota
	// FailureRequired propagates sink errors (wrapping [ErrAuditFailed]) so the
	// caller can fail a request that could not be recorded — "no audit, no
	// action", for regimes that must never act unrecorded.
	FailureRequired
)

// String implements [fmt.Stringer].
func (m FailureMode) String() string {
	switch m {
	case FailureBestEffort:
		return "best_effort"
	case FailureRequired:
		return "required"
	default:
		return "unknown"
	}
}

// Default recording and failure policies.
const (
	// DefaultPolicyMode records only denials.
	DefaultPolicyMode = PolicyDenyOnly
	// DefaultFailureMode never lets a failed audit write block a request.
	DefaultFailureMode = FailureBestEffort
)

// AttributeSanitizer transforms a single [Decision.Attributes] entry before it
// reaches the [Sink]. It receives the attribute key and value and returns the
// value to record: return the value unchanged to keep it, or a redacted
// placeholder to scrub it. Keys are never altered, so event structure is
// preserved. Configure with [WithAttributeSanitizer]; the built-in
// [RedactSensitiveAttributes] covers the common secret-bearing keys.
type AttributeSanitizer func(key, value string) string

// RedactedAttributePlaceholder is the value [RedactSensitiveAttributes]
// substitutes for a sensitive attribute.
const RedactedAttributePlaceholder = "[REDACTED]"

// DefaultSensitiveAttributeKeys are substrings that mark an attribute key as
// secret-bearing. Matching is case-insensitive substring, so "authorization"
// matches "Authorization" and "x-authorization". Extend the slice to cover
// domain-specific keys, or write a custom [AttributeSanitizer].
var DefaultSensitiveAttributeKeys = []string{
	"password", "passwd", "secret", "token", "authorization",
	"apikey", "api_key", "credential", "cookie", "session", "private_key",
}

// RedactSensitiveAttributes is an [AttributeSanitizer] that replaces the value of
// any attribute whose key contains one of [DefaultSensitiveAttributeKeys]
// (case-insensitive) with [RedactedAttributePlaceholder], leaving other values
// untouched.
func RedactSensitiveAttributes(key, value string) string {
	lower := strings.ToLower(key)
	for _, s := range DefaultSensitiveAttributeKeys {
		if strings.Contains(lower, s) {
			return RedactedAttributePlaceholder
		}
	}
	return value
}

// WithAttributeSanitizer sets a per-attribute sanitizer applied to every
// [Decision.Attributes] entry before it reaches the [Sink]. It is opt-in: with no
// sanitizer configured, attributes are recorded verbatim. The caller's map is
// never mutated — a sanitized copy is passed to the sink. Pass
// [RedactSensitiveAttributes] to scrub common secret-bearing keys, or a custom
// func for other policies.
//
//	rec := audit.NewRecorder(sink, audit.WithAttributeSanitizer(audit.RedactSensitiveAttributes))
func WithAttributeSanitizer(s AttributeSanitizer) Option {
	return func(o *options) {
		o.attributeSanitizer = s
	}
}

// options carries the recorder tunables.
type options struct {
	policyMode         PolicyMode         `optgen:"default=DefaultPolicyMode"`
	failureMode        FailureMode        `optgen:"default=DefaultFailureMode"`
	attributeSanitizer AttributeSanitizer `opt:"-"`
}
