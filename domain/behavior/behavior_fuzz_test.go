// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package behavior_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/behavior"
)

// FuzzParseKindIsExactAndTotal pins the token table: a token is recognized only
// if it is spelled exactly, and a recognized one round-trips.
//
// ParseKind reads the `behavior:"..."` struct tags that decide which fields a
// request may set and which the server strips. A token matched loosely — case
// folded, trimmed, prefix-matched — would silently grant a behavior nobody
// wrote, and a Kind that does not render back to its own token would make the
// tag unreadable in reverse.
func FuzzParseKindIsExactAndTotal(f *testing.F) {
	f.Add("required")
	f.Add("output_only")
	f.Add("OUTPUT_ONLY")
	f.Add(" required ")
	f.Add("required,")
	f.Add("")
	f.Add("-")
	f.Add("unspecified")
	f.Add("requiredx")
	f.Add(strings.Repeat("a", 128))

	f.Fuzz(func(t *testing.T, token string) {
		kind, ok := behavior.ParseKind(token)
		if !ok {
			require.Equal(t, behavior.Unspecified, kind,
				"a rejected token must not also yield a kind")
			return
		}

		require.NotEqual(t, behavior.Unspecified, kind,
			"Unspecified is not a behavior a tag can name")
		require.Equal(t, token, kind.String(),
			"a recognized token must be exactly how the kind renders itself")

		// Exact means exact: no neighboring spelling may resolve to a kind
		// unless it is a token in its own right.
		for _, variant := range []string{
			token + " ",
			" " + token,
			strings.ToUpper(token),
			token + "x",
		} {
			if variant == token {
				continue
			}
			if _, variantOK := behavior.ParseKind(variant); variantOK {
				require.Equal(t, variant, mustKind(t, variant).String(),
					"%q resolved to a kind whose own token is different", variant)
			}
		}
	})
}

// mustKind parses a token this test has already established is recognized.
func mustKind(t *testing.T, token string) behavior.Kind {
	t.Helper()

	kind, ok := behavior.ParseKind(token)
	require.True(t, ok)

	return kind
}
