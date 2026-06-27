// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scope_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/scope"
)

func TestExact(t *testing.T) {
	t.Parallel()

	m := scope.Exact()
	tests := []struct {
		name     string
		granted  []string
		required string
		want     bool
	}{
		{"present", []string{"files:read", "files:write"}, "files:read", true},
		{"absent", []string{"files:read"}, "files:write", false},
		{"empty granted", nil, "files:read", false},
		{"wildcard is literal", []string{"files:*"}, "files:read", false},
		{"star is literal", []string{"*"}, "files:read", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, m(tc.granted, tc.required))
		})
	}
}

func TestWildcard(t *testing.T) {
	t.Parallel()

	m := scope.Wildcard(":")
	tests := []struct {
		name     string
		granted  []string
		required string
		want     bool
	}{
		{"exact still matches", []string{"files:read"}, "files:read", true},
		{"prefix wildcard", []string{"files:*"}, "files:read", true},
		{"prefix wildcard deep", []string{"files:*"}, "files:write:bulk", true},
		{"global star", []string{"*"}, "anything:here", true},
		{"different domain denied", []string{"files:*"}, "buckets:read", false},
		{"boundary respected", []string{"files:*"}, "filesx:read", false},
		{"no wildcard no match", []string{"files"}, "files:read", false},
		{"empty granted", nil, "files:read", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, m(tc.granted, tc.required))
		})
	}
}
