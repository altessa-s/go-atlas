// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Internal test package: these tests access the unexported set/enabled
// dispatch functions to exercise the platform split without requiring
// CGO or syscall interception.
package nonewprivs

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSet_NonLinuxReturnsUnsupported verifies that the non-Linux stub
// returns ErrUnsupported. On Linux this test is a no-op because calling
// Set would permanently commit the test binary to NO_NEW_PRIVS and
// would break every subsequent test in the same process.
func TestSet_NonLinuxReturnsUnsupported(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("Set is irreversible on Linux; skipping to avoid poisoning the test binary")
	}
	err := Set()
	require.ErrorIs(t, err, ErrUnsupported)
}

// TestEnabled_NonLinuxReturnsUnsupported verifies the non-Linux stub for
// [Enabled]. On Linux, [Enabled] is safe to call at any time, but we
// still assert that it returns no error and a deterministic bool — the
// exact value depends on whether the test runner already set the bit,
// so we only require the call to succeed.
func TestEnabled_NonLinuxReturnsUnsupported(t *testing.T) {
	v, err := Enabled()
	if runtime.GOOS == "linux" {
		require.NoError(t, err, "Enabled on Linux")
		_ = v // value depends on ambient process state
		return
	}
	require.ErrorIs(t, err, ErrUnsupported)
	require.False(t, v, "Enabled: got true, want false on non-Linux")
}

// TestErrFailed_IsDistinctFromErrUnsupported is a trivial guard against
// a future refactor that accidentally collapses the two sentinels into
// one. Callers rely on [errors.Is] distinguishing them.
func TestErrFailed_IsDistinctFromErrUnsupported(t *testing.T) {
	require.NotErrorIs(t, ErrFailed, ErrUnsupported)
	require.NotErrorIs(t, ErrUnsupported, ErrFailed)
}
