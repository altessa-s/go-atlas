// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package topics_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/broker/providers/nats/topics"
)

// template is the subject shape the targets substitute into: two macros with a
// literal token between them, so a value that smuggled in a separator would
// change both the token count and which tokens a subscriber matches.
const template topics.Topic = "events.{TENANT}.orders.{ID}"

// valueSeeds are macro values worth starting from — ordinary identifiers, and
// the NATS metacharacters that decide subject structure.
var valueSeeds = []string{
	"acme",
	"",
	".",
	"*",
	">",
	"a.b",
	"a*b",
	"a>b",
	"..",
	"acme.>",
	"*.*",
	strings.Repeat("a", 300),
}

// FuzzWithNeverWidensTheSubject is the subject-injection oracle.
//
// A NATS subject is structural: "." separates tokens, "*" matches one token and
// ">" matches every remaining one. A macro value that carries any of them stops
// being data and becomes scope — a publisher reaching subjects it was never
// given, or a subscriber receiving another tenant's traffic from a single
// crafted identifier.
//
// The claim being pinned is the one [Topic.With] documents: values are
// sanitized, so the substituted subject keeps the template's token count and a
// token becomes a wildcard only when the value was *exactly* [topics.Wildcard]
// or [topics.FullWildcard] — the documented way for a caller to ask for a
// subscription pattern on purpose. Anything short of an exact match must be
// flattened, which is what makes "acme.>" data rather than scope.
func FuzzWithNeverWidensTheSubject(f *testing.F) {
	for _, seed := range valueSeeds {
		f.Add(seed, "id-1")
		f.Add("acme", seed)
		f.Add(seed, seed)
	}

	templateTokens := strings.Count(string(template), ".") + 1

	f.Fuzz(func(t *testing.T, tenant, id string) {
		subject := template.With("TENANT", tenant, "ID", id)

		require.Equal(t, templateTokens, strings.Count(subject, ".")+1,
			"a macro value introduced a token separator: tenant=%q id=%q → %q", tenant, id, subject)

		// The template's own tokens are literals; the other two came from the
		// values, in order.
		tokens := strings.Split(subject, ".")
		for token, value := range map[string]string{tokens[1]: tenant, tokens[3]: id} {
			if token != topics.Wildcard && token != topics.FullWildcard {
				continue
			}
			require.Equal(t, token, value,
				"a value that was not the literal wildcard became one: %q → %q", value, subject)
		}
	})
}

// FuzzWithValidationAcceptsOnlyWellFormedSubjects covers the entry point the
// package documents for untrusted input.
//
// WithValidation adds what With cannot: a length cap and wildcard positioning.
// The property is that anything it accepts is a subject NATS would take —
// bounded, and with ">" only in the final token, since a ">" anywhere else is a
// malformed subject the server rejects at subscribe time rather than a scope the
// caller intended.
func FuzzWithValidationAcceptsOnlyWellFormedSubjects(f *testing.F) {
	for _, seed := range valueSeeds {
		f.Add(seed, "id-1")
		f.Add("acme", seed)
		f.Add(seed, seed)
	}

	f.Fuzz(func(t *testing.T, tenant, id string) {
		subject, err := template.WithValidation("TENANT", tenant, "ID", id)
		if err != nil {
			require.Empty(t, subject, "a rejected substitution must not also produce a subject")
			return
		}

		require.LessOrEqual(t, len(subject), topics.MaxSubjectLength)

		tokens := strings.Split(subject, ".")
		for i, token := range tokens {
			if token == ">" {
				require.Equal(t, len(tokens)-1, i,
					"a full wildcard outside the final token: %q", subject)
			}
		}
	})
}

// FuzzWithIsDeterministic pins that substitution depends on nothing but its
// inputs — the same values must always render the same subject.
//
// Subjects are used as routing keys and as deduplication keys, so a rendering
// that varied between two calls would send otherwise identical messages to
// different places, which is the kind of drift nobody attributes to the
// template renderer.
func FuzzWithIsDeterministic(f *testing.F) {
	for _, seed := range valueSeeds {
		f.Add(seed, seed)
	}

	f.Fuzz(func(t *testing.T, tenant, id string) {
		first := template.With("TENANT", tenant, "ID", id)
		second := template.With("TENANT", tenant, "ID", id)
		require.Equal(t, first, second)
	})
}
