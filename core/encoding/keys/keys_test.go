// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package keys_test

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/encoding/keys"
)

// TestComposeABIPinned pins the bytes: composed keys persist, so the expected values are literals on purpose.
func TestComposeABIPinned(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		parts []string
		want  string
	}{
		{name: "no parts", parts: nil, want: ""},
		{name: "one empty part", parts: []string{""}, want: "0:"},
		{name: "colon in the first part", parts: []string{"a:b", "c"}, want: "3:a:b1:c"},
		{name: "colon in the second part", parts: []string{"a", "b:c"}, want: "1:a3:b:c"},
		{name: "multi-digit length", parts: []string{"0123456789"}, want: "10:0123456789"},
		{name: "unicode counts bytes", parts: []string{"κλειδί"}, want: "12:κλειδί"},
		{name: "binary bytes unchanged", parts: []string{"\x00\xff:"}, want: "3:\x00\xff:"},
		{name: "nested", parts: []string{"x", "3:a:b1:c"}, want: "1:x8:3:a:b1:c"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, keys.Compose(tc.parts...))
		})
	}
}

func TestComposeDistinctTuples(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ a, b []string }{
		{a: []string{"a:b", "c"}, b: []string{"a", "b:c"}},
		{a: []string{"", "a"}, b: []string{"a", ""}},
		{a: []string{"ab"}, b: []string{"a", "b"}},
		{a: nil, b: []string{""}},
		{a: []string{"1:a"}, b: []string{"a"}},
		{a: []string{"1:a1:b"}, b: []string{"a", "b"}},
	} {
		require.NotEqual(t, keys.Compose(tc.a...), keys.Compose(tc.b...), "%q vs %q", tc.a, tc.b)
	}
}

// TestDigestABIPinned pins the digest bytes: digests persist (idempotency request fingerprints).
func TestDigestABIPinned(t *testing.T) {
	t.Parallel()

	// sha256 of the empty input; then of 8 zero bytes (one empty part).
	require.Equal(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", hex.EncodeToString(keys.Digest()))
	require.Equal(t, "af5570f5a1810b7af78caf4bc70a660f0df51e42baf91d4de5b2328de0e83dfc", hex.EncodeToString(keys.Digest(nil)))
	// 8-byte big-endian length + bytes, per part.
	require.Equal(t, "42d472f330614fbf9742e89f974a3e2efe91f2481ed9eb8fa74574fbc5afb85e",
		hex.EncodeToString(keys.Digest([]byte("create"), []byte("a"))))
	require.Len(t, keys.Digest([]byte("a")), 32)
}

func TestDigestFraming(t *testing.T) {
	t.Parallel()

	require.Equal(t, keys.Digest([]byte("a"), []byte("b")), keys.Digest([]byte("a"), []byte("b")))
	require.NotEqual(t, keys.Digest([]byte("ab"), []byte("c")), keys.Digest([]byte("a"), []byte("bc")), "parts are length-prefixed")
	require.NotEqual(t, keys.Digest(), keys.Digest(nil), "an empty part counts")
}
