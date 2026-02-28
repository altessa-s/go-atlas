// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natsprovider

import (
	"regexp"
	"strings"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

var subjectRx = regexp.MustCompile(`^[a-zA-Z0-9\-]+$`)

// IsValidSubject reports whether subject contains only alphanumeric characters (a-z, A-Z, 0-9)
// and hyphens. It does not permit dots, wildcards, or whitespace.
//
// Example:
//
//	natsprovider.IsValidSubject("my-topic")   // true
//	natsprovider.IsValidSubject("orders.>")   // false
func IsValidSubject(subject string) bool {
	return subjectRx.MatchString(subject)
}

// subjectMatchesPattern reports whether a NATS subject matches a pattern.
//   - "*" matches exactly one token (tokens are separated by ".").
//   - ">" matches one or more tokens and must be the last token.
//   - Any other token matches literally.
func subjectMatchesPattern(subject, pattern string) bool {
	subTokens := strings.Split(subject, ".")
	patTokens := strings.Split(pattern, ".")

	for i, pt := range patTokens {
		if pt == ">" {
			// ">" must be last token and subject must have at least this many tokens.
			return i < len(subTokens)
		}
		if i >= len(subTokens) {
			return false
		}
		if pt != "*" && pt != subTokens[i] {
			return false
		}
	}
	return len(subTokens) == len(patTokens)
}

// checkSubjectAllowed returns an error if an allowlist is configured and the
// subject does not match any of the allowed patterns.
func (n *Nats) checkSubjectAllowed(subject string) error {
	if len(n.allowedSubjects) == 0 {
		return nil
	}
	for _, pattern := range n.allowedSubjects {
		if subjectMatchesPattern(subject, pattern) {
			return nil
		}
	}
	return coreerrs.Wrapf(ErrSubjectNotAllowed, "%q", subject)
}
