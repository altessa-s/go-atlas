// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natskey_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/internal/natskey"
)

// keyPattern is the character set nats.go accepts in a KeyValue key.
var keyPattern = regexp.MustCompile(`^[-/_=.a-zA-Z0-9]+$`)

// requireServerAccepts asserts what nats.go checks client-side (character set,
// no leading or trailing dot) plus what the server checks (no empty token).
func requireServerAccepts(tb testing.TB, key string) {
	tb.Helper()
	require.Regexp(tb, keyPattern, key)
	require.False(tb, strings.HasPrefix(key, "."), "leading dot in %q", key)
	require.False(tb, strings.HasSuffix(key, "."), "trailing dot in %q", key)
	require.NotContains(tb, key, "..", "empty token in %q", key)
}

func TestEncode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  string
		want string
	}{
		{name: "Empty", key: "", want: "="},
		{name: "Plain", key: "plain-key_1/v2.x", want: "plain-key_1/v2.x"},
		{name: "Colons", key: "idk:POST:/orders:123", want: "idk=3APOST=3A/orders=3A123"},
		{name: "IPv6", key: "2001:db8::1", want: "2001=3Adb8=3A=3A1"},
		{name: "EqualsIsEscaped", key: "a=3Ab", want: "a=3D3Ab"},
		{name: "LoneEquals", key: "=", want: "=3D"},
		{name: "LeadingAndTrailingDots", key: ".leading.", want: "=2Eleading=2E"},
		{name: "ConsecutiveDots", key: "a..b", want: "a.=2Eb"},
		{name: "OnlyDots", key: "..", want: "=2E=2E"},
		{name: "SingleDot", key: ".", want: "=2E"},
		{name: "Wildcards", key: "a*b>c", want: "a=2Ab=3Ec"},
		{name: "Whitespace", key: "a b\tc", want: "a=20b=09c"},
		{name: "NUL", key: "a\x00b", want: "a=00b"},
		{name: "Unicode", key: "κλειδί", want: "=CE=BA=CE=BB=CE=B5=CE=B9=CE=B4=CE=AF"},
		{name: "MalformedUTF8", key: "\xff\xfe", want: "=FF=FE"},
		{name: "Slashes", key: "//a//", want: "//a//"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := natskey.Encode(tc.key)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
			requireServerAccepts(t, got)

			back, err := natskey.Decode(got)
			require.NoError(t, err)
			require.Equal(t, tc.key, back)
		})
	}
}

func TestEncode_EscapedAndEscapeLookingKeysDoNotCollide(t *testing.T) {
	t.Parallel()

	a, err := natskey.Encode("a:b")
	require.NoError(t, err)
	b, err := natskey.Encode("a=3Ab")
	require.NoError(t, err)
	require.NotEqual(t, a, b)
}

func TestEncode_LengthBudget(t *testing.T) {
	t.Parallel()

	t.Run("LiteralAtLimit", func(t *testing.T) {
		t.Parallel()
		key := strings.Repeat("a", natskey.MaxLen)
		got, err := natskey.Encode(key)
		require.NoError(t, err)
		require.Equal(t, key, got)
	})
	t.Run("LiteralOverLimit", func(t *testing.T) {
		t.Parallel()
		_, err := natskey.Encode(strings.Repeat("a", natskey.MaxLen+1))
		require.ErrorIs(t, err, natskey.ErrKeyTooLong)
	})
	t.Run("EscapedAtLimit", func(t *testing.T) {
		t.Parallel()
		got, err := natskey.Encode(strings.Repeat(":", natskey.MaxLen/3))
		require.NoError(t, err)
		require.Len(t, got, natskey.MaxLen)
	})
	t.Run("EscapedOverLimit", func(t *testing.T) {
		t.Parallel()
		_, err := natskey.Encode(strings.Repeat(":", natskey.MaxLen/3) + "a")
		require.ErrorIs(t, err, natskey.ErrKeyTooLong)
	})
}

func TestEncode_ValidKeyIsReturnedWithoutAllocating(t *testing.T) {
	key := "tenant-1/orders.v2"
	allocs := testing.AllocsPerRun(100, func() {
		_, _ = natskey.Encode(key)
	})
	require.Zero(t, allocs)
}

func TestDecode_RejectsWhatEncodeNeverProduces(t *testing.T) {
	t.Parallel()

	for _, physical := range []string{
		"",        // empty
		"a:b",     // unescaped invalid byte
		"=3",      // truncated escape
		"=zz",     // not hex
		"=3a",     // lower-case hex
		"=41",     // escaped literal 'A'
		".a",      // leading dot
		"a..b",    // empty token
		"a=2Eb",   // escaped dot where a literal one fits
		"a.=2E.b", // a.=2E=2Eb is the encoding of "a...b"
	} {
		_, err := natskey.Decode(physical)
		require.ErrorIs(t, err, natskey.ErrMalformed, physical)
	}
}
