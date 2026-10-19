// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/idempotency"
)

// TestComposeKeyABIPinned pins the v2 storage-key bytes: keys persist in
// storage backends, so the expected values are hard-coded literals on
// purpose.
func TestComposeKeyABIPinned(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		parts []string
		want  string
	}{
		{name: "no components", parts: nil, want: "idk:v2:"},
		{name: "one empty component", parts: []string{""}, want: "idk:v2:0:"},
		{name: "colon in the first component", parts: []string{"a:b", "c"}, want: "idk:v2:3:a:b1:c"},
		{name: "colon in the second component", parts: []string{"a", "b:c"}, want: "idk:v2:1:a3:b:c"},
		{name: "empty namespace", parts: []string{"namespace", "", "op-1"}, want: "idk:v2:9:namespace0:4:op-1"},
		{name: "multi-digit length", parts: []string{"0123456789"}, want: "idk:v2:10:0123456789"},
		{name: "unicode counts bytes", parts: []string{"κλειδί"}, want: "idk:v2:12:κλειδί"},
		{name: "binary bytes unchanged", parts: []string{"\x00\xff:"}, want: "idk:v2:3:\x00\xff:"},
		{
			name:  "transport key",
			parts: []string{"transport", "POST:/api/v1/users", "550e8400-e29b-41d4-a716-446655440000"},
			want:  "idk:v2:9:transport18:POST:/api/v1/users36:550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name:  "transport key nested in a namespace",
			parts: []string{"namespace", "acme", "idk:v2:9:transport12:POST:/orders1:k"},
			want:  "idk:v2:9:namespace4:acme36:idk:v2:9:transport12:POST:/orders1:k",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, idempotency.ComposeKey(tc.parts...))
		})
	}
}

// TestComposeKeyDistinctTuples pins that tuples whose ':'-joined or
// concatenated forms are equal still compose to distinct keys.
func TestComposeKeyDistinctTuples(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a, b []string
	}{
		{name: "colon moves between components", a: []string{"a:b", "c"}, b: []string{"a", "b:c"}},
		{name: "empty moves between components", a: []string{"", "a"}, b: []string{"a", ""}},
		{name: "split vs joined", a: []string{"ab"}, b: []string{"a", "b"}},
		{name: "no components vs one empty", a: nil, b: []string{""}},
		{name: "component holding a length prefix", a: []string{"1:a"}, b: []string{"a"}},
		{name: "component holding a composed tuple", a: []string{"1:a1:b"}, b: []string{"a", "b"}},
		{name: "domain tags", a: []string{"transport", "s", "k"}, b: []string{"ibk", "s", "k"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.NotEqual(t, idempotency.ComposeKey(tc.a...), idempotency.ComposeKey(tc.b...))
		})
	}
}

// decomposeKey parses a key composed by [idempotency.ComposeKey] back into its
// components, reporting false for bytes ComposeKey cannot produce.
func decomposeKey(key string) ([]string, bool) {
	rest, ok := strings.CutPrefix(key, "idk:v2:")
	if !ok {
		return nil, false
	}
	var parts []string
	for rest != "" {
		digits, tail, ok := strings.Cut(rest, ":")
		if !ok || digits == "" || (len(digits) > 1 && digits[0] == '0') {
			return nil, false
		}
		n, err := strconv.ParseUint(digits, 10, 64)
		if err != nil || n > uint64(len(tail)) {
			return nil, false
		}
		parts = append(parts, tail[:n])
		rest = tail[n:]
	}
	return parts, true
}

func TestDecomposeKeyRoundTrip(t *testing.T) {
	t.Parallel()

	parts := []string{"namespace", "", "idk:v2:1:a3:b:c", "\x00κλειδί"}
	got, ok := decomposeKey(idempotency.ComposeKey(parts...))
	require.True(t, ok)
	require.Equal(t, parts, got)
}
