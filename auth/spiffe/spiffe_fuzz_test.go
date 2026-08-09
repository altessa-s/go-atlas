// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package spiffe_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/spiffe"
)

// FuzzParseID feeds arbitrary strings to the SPIFFE ID parser.
//
// The input is a URI SAN lifted out of a peer's certificate, so it is chosen by
// whoever holds that certificate — and the parsed trust domain is what an
// authorization decision is then made against. That makes the interesting
// failure not a panic but a disagreement: url.Parse is permissive, the checks
// after it are structural, and anything the first accepts that the second does
// not inspect ends up in TrustDomain.
//
// The assertions restate what a caller is entitled to assume from a non-error
// return, independently of how ParseID arrives at it.
func FuzzParseID(f *testing.F) {
	f.Add("spiffe://example.org/ns/default/sa/billing")
	f.Add("spiffe://example.org")
	f.Add("SPIFFE://Example.ORG/Path")
	f.Add("spiffe://user:pass@example.org/path")
	f.Add("spiffe://example.org:8443/path")
	f.Add("spiffe://example.org/path?query=1")
	f.Add("spiffe://example.org/path#fragment")
	f.Add("https://example.org/path")
	f.Add("spiffe://")
	f.Add("")
	f.Add("spiffe:///path")
	f.Add("spiffe://example.org/%2e%2e/other")
	f.Add("spiffe://ex%41mple.org/path")
	f.Add("spiffe://example.org/путь")
	f.Add("spiffe://:")            // Regression: a bare colon reached the trust domain.
	f.Add("spiffe://example.org:") // Regression: an empty port reached the trust domain.
	f.Add("spiffe://0/%00")        // Regression: an accepted ID that String could not re-render.

	f.Fuzz(func(t *testing.T, raw string) {
		id, err := spiffe.ParseID(raw)
		if err != nil {
			require.True(t, id.IsZero(), "a rejected ID must not also be returned: %#v", id)
			return
		}

		require.NotEmpty(t, id.TrustDomain, "an accepted ID must name a trust domain")
		require.Equal(t, strings.ToLower(id.TrustDomain), id.TrustDomain,
			"the trust domain must be canonicalized to lowercase, or two spellings of one peer compare unequal")

		// The spec forbids these components; if one survived into the authority
		// it would let a caller present "attacker@trusted.example" or
		// "trusted.example:1" and be compared against a policy entry it does
		// not actually match.
		require.NotContains(t, id.TrustDomain, "@", "userinfo leaked into the trust domain")
		require.NotContains(t, id.TrustDomain, ":", "a port leaked into the trust domain")
		require.NotContains(t, id.TrustDomain, "/", "a path separator leaked into the trust domain")
		require.NotContains(t, id.Path, "?", "a query leaked into the path")
		require.NotContains(t, id.Path, "#", "a fragment leaked into the path")

		if id.Path != "" {
			require.Equal(t, "/", id.Path[:1], "a non-empty path must keep its leading slash")
		}
	})
}

// FuzzParseIDIsIdempotent pins the canonical form: rendering an accepted ID and
// parsing it again must land on the same ID.
//
// This is where percent-encoding, case folding and empty-path handling would
// show up. An ID that parses to something whose String() parses to something
// else means two representations of "the same" workload exist, and a system that
// stores one and compares against the other silently stops matching — or starts
// matching something it should not.
func FuzzParseIDIsIdempotent(f *testing.F) {
	f.Add("spiffe://example.org/ns/default/sa/billing")
	f.Add("spiffe://example.org")
	f.Add("SPIFFE://Example.ORG/Path")
	f.Add("spiffe://example.org/%2e%2e/other")
	f.Add("spiffe://ex%41mple.org/path")
	f.Add("spiffe://example.org//double//slash")

	f.Fuzz(func(t *testing.T, raw string) {
		id, err := spiffe.ParseID(raw)
		if err != nil {
			return // Rejection is a fine outcome; only accepted IDs carry this obligation.
		}

		again, err := spiffe.ParseID(id.String())
		require.NoError(t, err, "an ID this package rendered must be one it accepts: %q", id.String())
		require.Equal(t, id, again, "the canonical form of %q is not stable", raw)
	})
}
