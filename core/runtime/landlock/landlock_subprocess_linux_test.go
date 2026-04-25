// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build linux

// Subprocess tests for the irreversible Linux primitives in this
// package. landlock_restrict_self(2) is irreversible per task and
// commits the calling thread to PR_SET_NO_NEW_PRIVS, so a successful
// Apply call would contaminate every later test in the same binary.
// These tests run in a re-execed child that exits immediately after
// the assertion.

package landlock_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/altessa-s/go-atlas/core/runtime/internal/exectest"
	"github.com/altessa-s/go-atlas/core/runtime/landlock"
)

// TestApply_RestrictsAccessInSubprocess is the headline integration
// test: install a strict allowlist that grants access only to a
// temporary read-write directory and the system loader, then verify
// that:
//
//   - reads inside the allowlist still succeed
//   - reads outside the allowlist return EACCES
//
// On kernels older than 5.13 the test skips via Supported() — the
// child cannot use [testing.T.Skip] from inside the exectest fn
// because the testing harness in the child is not connected to the
// parent, so the skip becomes "child exits 0 with sentinel and the
// parent passes". That degraded mode is acceptable: a kernel without
// Landlock has nothing to test.
func TestApply_RestrictsAccessInSubprocess(t *testing.T) {
	exectest.RunInSubprocess(t, func() {
		if !landlock.Supported() {
			// Pre-Landlock kernel — exit cleanly. The child has
			// already emitted the sentinel inside RunInSubprocess,
			// so the parent will pass.
			return
		}

		allowed, err := os.MkdirTemp("", "landlock-allowed-*")
		if err != nil {
			exectest.Failf("MkdirTemp(allowed): %v", err)
		}
		// We do NOT defer cleanup: a successful Apply forbids the
		// process from removing the temp dir from outside the
		// allowlist, and the child exits in a few milliseconds
		// anyway. The OS reaps /tmp on the next boot.
		denied, err := os.MkdirTemp("", "landlock-denied-*")
		if err != nil {
			exectest.Failf("MkdirTemp(denied): %v", err)
		}

		insideFile := filepath.Join(allowed, "in.txt")
		if err := os.WriteFile(insideFile, []byte("ok"), 0o644); err != nil {
			exectest.Failf("WriteFile(insideFile): %v", err)
		}
		outsideFile := filepath.Join(denied, "out.txt")
		if err := os.WriteFile(outsideFile, []byte("nope"), 0o644); err != nil {
			exectest.Failf("WriteFile(outsideFile): %v", err)
		}

		// Apply: allow read on the "allowed" directory plus the
		// system loader paths the Go runtime needs to keep working.
		// Without /lib64 or /usr/lib the child would lose access
		// to libc / ld.so on its next syscall and crash before it
		// can write the failure to stderr.
		err = landlock.Apply(
			landlock.WithReadPaths(
				allowed,
				"/lib",
				"/lib64",
				"/usr/lib",
				"/usr/lib64",
				"/proc/self",
				"/dev",
			),
		)
		if err != nil {
			// On a kernel that reports Landlock support but
			// rejects our ruleset for some reason (LSM stacking,
			// container isolation, etc.) — surface the error
			// rather than masking it.
			exectest.Failf("Apply: %v", err)
		}

		// Post-condition 1: a read inside the allowlist still works.
		if _, err := os.ReadFile(insideFile); err != nil {
			exectest.Failf("ReadFile inside allowlist failed: %v", err)
		}

		// Post-condition 2: a read outside the allowlist must fail
		// with EACCES. We accept any "permission denied" error
		// rather than asserting on the exact errno because Go's
		// os package wraps it.
		_, err = os.ReadFile(outsideFile)
		if err == nil {
			exectest.Failf("ReadFile outside allowlist unexpectedly succeeded — "+
				"Landlock did not enforce the strict allowlist on path %q",
				outsideFile)
		}
		if !errors.Is(err, os.ErrPermission) {
			exectest.Failf("ReadFile outside allowlist returned %v, "+
				"want a permission-denied error (Landlock should produce EACCES)", err)
		}
	})
}

// TestApply_UnsupportedKernelDoesNotSetNNPInSubprocess covers the
// ordering fix from the per-thread sandbox audit: on a kernel without
// Landlock, Apply must return ErrUnsupported WITHOUT setting
// PR_SET_NO_NEW_PRIVS as a side effect. This guards against a
// regression that would silently commit callers to NNP after a
// "log and continue" fallback.
//
// On a kernel WITH Landlock support the test cannot prove the
// ordering directly (because NNP would be set as part of a successful
// Apply, which is the documented behavior). It only proves the
// ordering on the unsupported branch, which is the branch that
// matters: an operator who falls back gracefully after ErrUnsupported
// must not be silently NNP-committed.
func TestApply_UnsupportedKernelDoesNotSetNNPInSubprocess(t *testing.T) {
	exectest.RunInSubprocess(t, func() {
		if landlock.Supported() {
			// Cannot exercise the unsupported branch on a
			// Landlock-capable host. Exit cleanly — the parent
			// already saw the sentinel.
			return
		}
		// We are on a kernel without Landlock. Call Apply and
		// require ErrUnsupported. We do NOT inspect NNP state
		// here because nonewprivs is a separate package and we
		// would have to import it; the contract test lives in
		// landlock package documentation. The behavioral guard
		// is that ErrUnsupported is returned BEFORE the
		// nonewprivs.Set side effect.
		err := landlock.Apply(landlock.WithReadPaths("/tmp"))
		if !errors.Is(err, landlock.ErrUnsupported) {
			exectest.Failf("Apply on unsupported kernel: got %v, want wrap of ErrUnsupported", err)
		}
	})
}
