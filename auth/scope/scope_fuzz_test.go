// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scope_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/scope"
)

// scopeSeeds are grants and requirements worth starting from: ordinary
// hierarchical scopes, and the near-misses a wildcard matcher gets wrong.
var scopeSeeds = []string{
	"files:read",
	"files:write:bulk",
	"files:*",
	"filesx:read",
	"*",
	"",
	":",
	":*",
	"files:",
	"files:**",
	"*:read",
	"files:*:read",
}

// FuzzWildcardGrantsNothingItWasNotGiven is the authorization oracle: a
// hierarchical matcher may be more permissive than exact equality, but only in
// the two ways it documents.
//
// The rule is restated here rather than reused from the implementation, because
// an oracle that shares code with its subject agrees with it even when both are
// wrong. Three grants match and nothing else does: the scope itself, the global
// "*", and a "prefix+sep+*" grant whose prefix the requirement actually starts
// with. The last is where the boundary lives — "files:*" must satisfy
// "files:read" and must not satisfy "filesx:read", and a matcher that compares
// prefixes without the separator gets exactly that wrong.
func FuzzWildcardGrantsNothingItWasNotGiven(f *testing.F) {
	for _, granted := range scopeSeeds {
		for _, required := range scopeSeeds {
			f.Add(granted, required, ":")
		}
	}
	f.Add("files/read", "files/*", "/")
	f.Add("a", "a", "")

	f.Fuzz(func(t *testing.T, granted, required, sep string) {
		if sep == "" {
			t.Skip("an empty separator makes every grant a prefix of everything, which is not a hierarchy")
		}

		match := scope.Wildcard(sep)([]scope.Scope{granted}, required)

		suffix := sep + "*"
		want := granted == required ||
			granted == "*" ||
			(strings.HasSuffix(granted, suffix) &&
				strings.HasPrefix(required, strings.TrimSuffix(granted, "*")))

		require.Equal(t, want, match,
			"Wildcard(%q) granted=%q required=%q", sep, granted, required)
	})
}

// FuzzWildcardIsAtLeastAsPermissiveAsExact pins the relationship between the two
// matchers: swapping Exact for Wildcard may widen access, never narrow it.
//
// A service that switches matchers to support hierarchical grants would
// otherwise silently start denying scopes its principals already held, and the
// symptom — some requests failing authorization after a config change — points
// nowhere near the matcher.
func FuzzWildcardIsAtLeastAsPermissiveAsExact(f *testing.F) {
	for _, granted := range scopeSeeds {
		for _, required := range scopeSeeds {
			f.Add(granted, granted, required)
		}
	}

	f.Fuzz(func(t *testing.T, first, second, required string) {
		grants := []scope.Scope{first, second}

		if !scope.Exact()(grants, required) {
			return
		}

		require.True(t, scope.Wildcard(":")(grants, required),
			"Wildcard denied a scope Exact granted: grants=%v required=%q", grants, required)
	})
}

// FuzzMatchersIgnoreGrantOrder pins that a decision does not depend on the order
// scopes happen to arrive in.
//
// Granted scopes come from a token claim, so their order is whatever the issuer
// serialized — and an authorization that flips with it is one that denies a
// request the same principal made successfully a minute earlier.
func FuzzMatchersIgnoreGrantOrder(f *testing.F) {
	for _, a := range scopeSeeds {
		for _, b := range scopeSeeds {
			f.Add(a, b, "files:read")
		}
	}

	f.Fuzz(func(t *testing.T, first, second, required string) {
		forward := []scope.Scope{first, second}
		reversed := []scope.Scope{second, first}

		require.Equal(t, scope.Exact()(forward, required), scope.Exact()(reversed, required),
			"Exact depends on grant order: %v vs %v for %q", forward, reversed, required)
		require.Equal(t, scope.Wildcard(":")(forward, required), scope.Wildcard(":")(reversed, required),
			"Wildcard depends on grant order: %v vs %v for %q", forward, reversed, required)
	})
}
