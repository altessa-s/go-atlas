// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natsprovider

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// FuzzSubjectMatchesPatternHonorsTokenBoundaries is the subject-allowlist
// oracle.
//
// checkSubjectAllowed decides whether a publisher may reach a subject, and it
// decides it with this matcher. NATS wildcards are per-token — "*" is exactly
// one token, ">" is one or more and only as the last — so a matcher that
// compared prefixes instead would let "orders.acme" through a rule written for
// "orders.a", which is a tenant boundary crossed by a string that merely looks
// similar.
//
// The rule is restated here independently of the implementation.
func FuzzSubjectMatchesPatternHonorsTokenBoundaries(f *testing.F) {
	f.Add("orders.acme.created", "orders.*.created")
	f.Add("orders.acme.created", "orders.>")
	f.Add("orders.acme", "orders.a")
	f.Add("orders", "orders.>")
	f.Add("orders.a.b", "orders.*")
	f.Add("", "")
	f.Add(".", "*.*")
	f.Add("a", ">")
	f.Add("a.b", "*.>")
	f.Add("a.b.c", "a.>.c")

	f.Fuzz(func(t *testing.T, subject, pattern string) {
		require.Equal(t, wantMatch(subject, pattern), subjectMatchesPattern(subject, pattern),
			"subject=%q pattern=%q", subject, pattern)
	})
}

// wantMatch is an independent statement of NATS subject matching: token by
// token, "*" consuming exactly one and ">" consuming the rest as the final
// pattern token.
func wantMatch(subject, pattern string) bool {
	subTokens := strings.Split(subject, ".")
	patTokens := strings.Split(pattern, ".")

	for i, pt := range patTokens {
		if pt == ">" {
			// ">" stands for one or more remaining tokens, so there must be at
			// least one left to consume.
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

// FuzzValidSubjectsMatchThemselves pins the relationship between the validator
// and the matcher: a subject the provider accepts must match itself.
//
// The two are used together — a subject is validated on the way in and matched
// against the allowlist afterwards — so a subject that passes validation but
// cannot match its own literal pattern would be publishable and unroutable at
// once, which surfaces as messages quietly going nowhere.
func FuzzValidSubjectsMatchThemselves(f *testing.F) {
	f.Add("my-topic")
	f.Add("")
	f.Add("topic.dots")
	f.Add("a-b-c-123")
	f.Add("a..b")
	f.Add("a.*.b")
	f.Add("a.>")

	f.Fuzz(func(t *testing.T, subject string) {
		if !IsValidSubject(subject) {
			return
		}

		require.True(t, subjectMatchesPattern(subject, subject),
			"a subject the provider accepts does not match its own literal pattern: %q", subject)
	})
}
