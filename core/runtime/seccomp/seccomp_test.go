// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Internal test package: the cross-platform tests live here and
// exercise only the sentinels and the platform-dispatch contract.
// Filter-construction tests live in seccomp_linux_test.go because
// buildFilter and the dangerousSyscalls list are Linux-specific.
package seccomp

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSentinels_AreDistinct guards against a future refactor that
// accidentally collapses the two sentinels into one. Callers rely on
// [errors.Is] distinguishing them so they can tell "platform can't
// do seccomp" from "kernel rejected the filter".
func TestSentinels_AreDistinct(t *testing.T) {
	require.NotErrorIs(t, ErrFailed, ErrUnsupported)
	require.NotErrorIs(t, ErrUnsupported, ErrFailed)
}

// TestBlockDangerousSyscalls_NonLinuxReturnsUnsupported verifies the
// stub contract on platforms we do not support. On Linux/amd64 and
// Linux/arm64 this test skips because calling install() would
// irreversibly add a filter to the test binary and poison every
// subsequent test in the process.
func TestBlockDangerousSyscalls_NonLinuxReturnsUnsupported(t *testing.T) {
	if runtime.GOOS == "linux" && (runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64") {
		t.Skip("BlockDangerousSyscalls is irreversible on supported platforms; skipping to avoid poisoning the test binary")
	}
	err := BlockDangerousSyscalls()
	require.ErrorIs(t, err, ErrUnsupported)
}
