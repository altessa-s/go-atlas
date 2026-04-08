// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build linux

// Subprocess tests for the irreversible Linux primitives in this
// package. They run in a re-execed test binary so the side effect
// (PR_SET_NO_NEW_PRIVS becomes set on the calling thread for the
// rest of the process's lifetime) is contained to a child that
// exits immediately after the assertion.
//
// See [github.com/altessa-s/go-atlas/core/runtime/internal/exectest]
// for the helper that re-execs the binary.

package nonewprivs_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/core/runtime/internal/exectest"
	"github.com/altessa-s/go-atlas/core/runtime/nonewprivs"
)

// TestSet_RoundTripInSubprocess covers the happy path of Set() →
// Enabled() in a subprocess. Before Set, Enabled may report either
// state depending on what the test runner inherited; after Set,
// Enabled must report true. The post-condition is what gives this
// test value: it proves the prctl actually landed on the kernel
// side, not just that the syscall returned nil.
func TestSet_RoundTripInSubprocess(t *testing.T) {
	exectest.RunInSubprocess(t, func() {
		// Pre-condition: read but do not assert. The CI runner may
		// have inherited NO_NEW_PRIVS from a wrapping container or
		// systemd unit, so a "must be false before Set" assertion
		// would be flaky in production CI.
		_, err := nonewprivs.Enabled()
		if err != nil {
			exectest.Failf("Enabled() before Set: %v", err)
		}

		if err := nonewprivs.Set(); err != nil {
			exectest.Failf("Set: %v", err)
		}

		// Post-condition: NNP must now report enabled on the
		// calling thread. Set is documented as per-thread, and
		// this test runs both Set and Enabled on the same
		// goroutine without intervening blocking syscalls, so the
		// Go runtime is highly unlikely to migrate us mid-test.
		// We do not pin via runtime.LockOSThread because we want
		// the test to fail if a real per-thread divergence
		// surfaces — that would be a regression worth catching.
		got, err := nonewprivs.Enabled()
		if err != nil {
			exectest.Failf("Enabled() after Set: %v", err)
		}
		if !got {
			exectest.Failf("Enabled() after Set returned false; " +
				"expected true (kernel did not honor PR_SET_NO_NEW_PRIVS)")
		}
	})
}

// TestSet_IdempotentInSubprocess verifies that calling Set twice in
// a row succeeds. The kernel allows redundant PR_SET_NO_NEW_PRIVS,
// and the documented contract on [Set] promises idempotence.
func TestSet_IdempotentInSubprocess(t *testing.T) {
	exectest.RunInSubprocess(t, func() {
		if err := nonewprivs.Set(); err != nil {
			exectest.Failf("first Set: %v", err)
		}
		if err := nonewprivs.Set(); err != nil {
			exectest.Failf("second Set: %v", err)
		}
		got, err := nonewprivs.Enabled()
		if err != nil {
			exectest.Failf("Enabled after double Set: %v", err)
		}
		if !got {
			exectest.Failf("Enabled() returned false after double Set")
		}
	})
}
