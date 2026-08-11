// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	grpcmetadata "google.golang.org/grpc/metadata"
)

// FuzzExtractBearerTokenMatchesTheSchemeExactly pins which Authorization values
// yield a token and what that token is.
//
// The extracted token is the credential every downstream check runs against, so
// the interesting failure is not a panic but a disagreement: a header the
// scheme should reject producing a token, or an accepted header producing
// something other than the bytes after the prefix. Either turns the difference
// between "Bearer abc" and something adjacent to it into an authentication
// decision nobody wrote down.
//
// The rule is restated independently below, because an oracle built from the
// implementation agrees with it even when both are wrong.
func FuzzExtractBearerTokenMatchesTheSchemeExactly(f *testing.F) {
	f.Add("Bearer valid-token")
	f.Add("bearer lowercase")
	f.Add("BEARER upper")
	f.Add("Bearer  double-space")
	f.Add("Bearer\ttab")
	f.Add("Basic abc")
	f.Add("")
	f.Add("BearerNoSpace")
	f.Add("Bearer ")
	f.Add(" Bearer leading-space")

	extractor := ExtractBearerToken()

	f.Fuzz(func(t *testing.T, header string) {
		ctx := grpcmetadata.NewIncomingContext(t.Context(),
			grpcmetadata.Pairs("authorization", header))

		token, err := extractor.ExtractToken(ctx)

		// gRPC metadata is transported lowercase and the extractor trims the
		// value before matching, so the comparison is against the same shape.
		value := strings.TrimSpace(header)
		const prefix = "bearer "
		accepted := len(value) >= len(prefix) && strings.EqualFold(value[:len(prefix)], prefix)

		if !accepted {
			require.Error(t, err, "a header outside the bearer scheme produced a token: %q", header)
			require.Empty(t, token, "a rejected header must not also yield a token")
			return
		}

		require.NoError(t, err, "a well-formed bearer header was rejected: %q", header)
		require.Equal(t, strings.TrimSpace(value[len(prefix):]), token,
			"the token is not the bytes after the scheme: %q", header)
	})
}

// FuzzExtractBearerTokenNeverReturnsAnEmptyCredential pins that "Bearer " with
// nothing after it is not a credential.
//
// An empty token is the shape most likely to be mishandled downstream — a
// validator that treats "" as "no token supplied" and falls through to an
// anonymous path, or a cache keyed on it. Rejecting it here means no
// downstream has to have an opinion.
func FuzzExtractBearerTokenNeverReturnsAnEmptyCredential(f *testing.F) {
	f.Add("Bearer ")
	f.Add("Bearer    ")
	f.Add("bearer\t")
	f.Add("Bearer x")

	extractor := ExtractBearerToken()

	f.Fuzz(func(t *testing.T, header string) {
		ctx := grpcmetadata.NewIncomingContext(t.Context(),
			grpcmetadata.Pairs("authorization", header))

		token, err := extractor.ExtractToken(ctx)
		if err != nil {
			return
		}

		require.NotEmpty(t, token,
			"an empty credential was extracted from %q, leaving every downstream check to decide what that means", header)
	})
}
