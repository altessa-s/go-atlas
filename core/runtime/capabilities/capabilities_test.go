// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Internal test package: these tests exercise the exported API plus
// the name registry directly. Mutating functions (DropAll,
// DropAllExcept, Apply, BoundingDrop, ambient*) are deliberately NOT
// exercised — calling them on the test binary would irreversibly
// mutate its capability set and poison every subsequent test. For
// real syscall coverage, run the subprocess smoke test documented in
// the package README.
package capabilities

import (
	"errors"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseName_Known(t *testing.T) {
	cases := []struct {
		in   string
		want Cap
	}{
		{"CAP_CHOWN", CAP_CHOWN},
		{"CAP_NET_BIND_SERVICE", CAP_NET_BIND_SERVICE},
		{"CAP_SYS_ADMIN", CAP_SYS_ADMIN},
		{"CAP_CHECKPOINT_RESTORE", CAP_CHECKPOINT_RESTORE},
		// Case-insensitive + whitespace tolerance.
		{"cap_net_raw", CAP_NET_RAW},
		{"  CAP_SETUID  ", CAP_SETUID},
		{"Cap_Bpf", CAP_BPF},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := ParseName(tc.in)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestParseName_Unknown(t *testing.T) {
	cases := []string{
		"",
		"CAP_DOES_NOT_EXIST",
		"NET_BIND_SERVICE", // missing CAP_ prefix
		"cap_net_bind_service_typo",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseName(name)
			require.Error(t, err)
			require.ErrorIs(t, err, ErrInvalidOption)
		})
	}
}

func TestCap_String_RoundTrip(t *testing.T) {
	for c := Cap(0); c <= capLastCap; c++ {
		name := c.String()
		got, err := ParseName(name)
		require.NoError(t, err, "ParseName(%q)", name)
		require.Equal(t, c, got, "round trip for %d", c)
	}
}

func TestCap_String_Unknown(t *testing.T) {
	// A Cap value beyond capLastCap should produce a synthetic
	// "CAP_UNKNOWN(N)" form rather than an empty string, so callers
	// logging an unrecognized cap still see something useful.
	require.Equal(t, "CAP_UNKNOWN(999)", Cap(999).String())
}

// TestSentinels_AreDistinct guards against a future refactor that
// accidentally collapses the three sentinels into one. Callers rely
// on [errors.Is] distinguishing them.
func TestSentinels_AreDistinct(t *testing.T) {
	pairs := [][2]error{
		{ErrFailed, ErrUnsupported},
		{ErrFailed, ErrInvalidOption},
		{ErrUnsupported, ErrInvalidOption},
	}
	for _, p := range pairs {
		require.False(t, errors.Is(p[0], p[1]), "%v should not match %v", p[0], p[1])
	}
}

// TestGet_PlatformDispatch verifies that [Get] dispatches to the
// platform-appropriate implementation. On Linux it must return a
// non-error snapshot (the exact contents depend on the ambient
// process state of the test runner). On non-Linux platforms it must
// return the zero-value Sets and ErrUnsupported.
func TestGet_PlatformDispatch(t *testing.T) {
	s, err := Get()
	if runtime.GOOS == "linux" {
		require.NoError(t, err, "Get on linux")
		_ = s
		return
	}
	require.ErrorIs(t, err, ErrUnsupported)
	require.Equal(t, Sets{}, s)
}

// TestCapConstants_ContiguousRange is a cheap sanity check that the
// constant block covers every value from CAP_CHOWN through
// capLastCap without gaps. A future edit that accidentally skips a
// number (e.g. defining CAP_X = 40 but forgetting 39) would silently
// break ParseName and (Cap).String for the missing bit.
func TestCapConstants_ContiguousRange(t *testing.T) {
	require.Len(t, capToName, int(capLastCap)+1)
	for c := Cap(0); c <= capLastCap; c++ {
		_, ok := capToName[c]
		require.True(t, ok, "capToName missing entry for Cap(%d)", c)
	}
}

// TestCapConstants_UniqueNames ensures nameToCap has the same number
// of entries as capToName — i.e. no two Cap values accidentally map
// to the same string.
func TestCapConstants_UniqueNames(t *testing.T) {
	require.Equal(t, len(capToName), len(nameToCap), "registry size mismatch")
}
