// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package behavior_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/behavior"
)

func TestParseKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		token string
		want  behavior.Kind
		ok    bool
	}{
		{"required", behavior.Required, true},
		{"output_only", behavior.OutputOnly, true},
		{"input_only", behavior.InputOnly, true},
		{"immutable", behavior.Immutable, true},
		{"identifier", behavior.Identifier, true},
		{"unspecified", behavior.Unspecified, false},
		{"", behavior.Unspecified, false},
		{"bogus", behavior.Unspecified, false},
	}

	for _, tc := range tests {
		t.Run(tc.token, func(t *testing.T) {
			t.Parallel()

			got, ok := behavior.ParseKind(tc.token)
			require.Equal(t, tc.ok, ok)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestKindStringOutOfRange(t *testing.T) {
	t.Parallel()

	require.Equal(t, "kind(99)", behavior.Kind(99).String())
}

func TestKindStringRoundTrip(t *testing.T) {
	t.Parallel()

	for _, b := range []behavior.Kind{
		behavior.Required,
		behavior.OutputOnly,
		behavior.InputOnly,
		behavior.Immutable,
		behavior.Identifier,
	} {
		parsed, ok := behavior.ParseKind(b.String())
		require.True(t, ok, "token %q should parse", b.String())
		require.Equal(t, b, parsed)
	}
}
