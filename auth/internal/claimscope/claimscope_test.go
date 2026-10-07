// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package claimscope_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/internal/claimscope"
)

func TestSeq(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   any
		want []string
	}{
		{"space separated string", "read write  admin", []string{"read", "write", "admin"}},
		{"surrounding whitespace", "  read\twrite\n", []string{"read", "write"}},
		{"empty string", "", nil},
		{"string slice", []string{"read", "write"}, []string{"read", "write"}},
		{"string slice keeps spaces", []string{"a b"}, []string{"a b"}},
		{"mixed any slice", []any{"read", 42, nil, "write", true}, []string{"read", "write"}},
		{"nil", nil, nil},
		{"unsupported type", 42, nil},
		{"unsupported map", map[string]any{"scope": "read"}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, slices.Collect(claimscope.Seq(tc.in)))
		})
	}
}

func TestSeqStopsEarly(t *testing.T) {
	t.Parallel()
	for _, in := range []any{"a b c", []string{"a", "b", "c"}, []any{"a", 1, "b", "c"}} {
		var got []string
		for s := range claimscope.Seq(in) {
			got = append(got, s)
			if len(got) == 2 {
				break
			}
		}
		require.Equal(t, []string{"a", "b"}, got)
	}
}
