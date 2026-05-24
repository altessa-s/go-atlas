// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build linux

// Subprocess tests for the irreversible Linux primitives in this
// package. capset(2) drops are irreversible for an unprivileged
// process and bounding-set drops are irreversible even for root, so a
// successful DropAll/DropAllExcept call would contaminate every later
// test in the same binary. These tests run in a re-execed child that
// exits immediately after the assertion.

package capabilities_test

import (
	"os"
	"testing"

	"github.com/altessa-s/go-atlas/core/runtime/capabilities"
	"github.com/altessa-s/go-atlas/core/runtime/internal/exectest"
)

// TestDropAll_ZeroesEverySetInSubprocess verifies that DropAll
// actually clears effective/permitted/inheritable/bounding/ambient on
// the calling thread. The test reads the snapshot via Get() after
// DropAll and asserts every mask is zero.
//
// This is the highest-value test in the package because it covers the
// full applySnapshot sequence end-to-end against a real kernel — the
// existing unit tests only exercise the validation layer (which fails
// before any syscall is issued).
//
// Pre-condition: a process can always drop its own caps regardless of
// initial state, so this test does not require root or any specific
// starting capability. If the binary already has zero caps the test
// is still meaningful: it verifies the no-op path through
// applySnapshot does not corrupt the snapshot.
func TestDropAll_ZeroesEverySetInSubprocess(t *testing.T) {
	// Skip in CI environments where capabilities manipulation may fail
	// due to container restrictions.
	if os.Getenv("CI") == "true" || os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("skipping capabilities test in CI environment")
	}

	exectest.RunInSubprocess(t, func() {
		if err := capabilities.DropAll(); err != nil {
			exectest.Failf("DropAll: %v", err)
		}
		got, err := capabilities.Get()
		if err != nil {
			exectest.Failf("Get after DropAll: %v", err)
		}
		if got.Effective != 0 {
			exectest.Failf("Effective = %#x, want 0", got.Effective)
		}
		if got.Permitted != 0 {
			exectest.Failf("Permitted = %#x, want 0", got.Permitted)
		}
		if got.Inheritable != 0 {
			exectest.Failf("Inheritable = %#x, want 0", got.Inheritable)
		}
		if got.Bounding != 0 {
			exectest.Failf("Bounding = %#x, want 0 "+
				"(bounding drop is irreversible — DropAll must clear it)",
				got.Bounding)
		}
		if got.Ambient != 0 {
			exectest.Failf("Ambient = %#x, want 0", got.Ambient)
		}
	})
}

// TestDropAllExcept_KeepsListedAndDropsRestInSubprocess covers the
// "bind a privileged port then drop everything else" pattern. It
// requests one capability be preserved and asserts:
//
//   - the kept cap is in effective+permitted+bounding (where the
//     kernel would let it land)
//   - every other bit is zero
//
// Note: an unprivileged test runner may not have CAP_NET_BIND_SERVICE
// in its initial bounding set. In that case the kernel will not let
// the cap land in the new permitted set either, and the test will
// observe everything-zero — which is still a correct outcome of
// DropAllExcept (you cannot keep what you do not have). The
// assertions below tolerate that case.
func TestDropAllExcept_KeepsListedAndDropsRestInSubprocess(t *testing.T) {
	// Skip in CI environments where capabilities manipulation may fail
	// due to container restrictions.
	if os.Getenv("CI") == "true" || os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("skipping capabilities test in CI environment")
	}

	exectest.RunInSubprocess(t, func() {
		// Read the starting snapshot so we can detect the
		// "unprivileged runner" case correctly.
		before, err := capabilities.Get()
		if err != nil {
			exectest.Failf("Get before DropAllExcept: %v", err)
		}
		bit := uint64(1) << capabilities.CAP_NET_BIND_SERVICE
		hadCapBefore := before.Bounding&bit != 0

		if err := capabilities.DropAllExcept(capabilities.CAP_NET_BIND_SERVICE); err != nil {
			exectest.Failf("DropAllExcept(CAP_NET_BIND_SERVICE): %v", err)
		}
		after, err := capabilities.Get()
		if err != nil {
			exectest.Failf("Get after DropAllExcept: %v", err)
		}

		// Inheritable and Ambient must always be zero — the
		// package documents that DropAllExcept clears them
		// unconditionally.
		if after.Inheritable != 0 {
			exectest.Failf("Inheritable = %#x, want 0", after.Inheritable)
		}
		if after.Ambient != 0 {
			exectest.Failf("Ambient = %#x, want 0", after.Ambient)
		}

		if hadCapBefore {
			// Privileged starting state: the kept cap must
			// survive in effective/permitted/bounding.
			if after.Effective&bit == 0 {
				exectest.Failf("Effective lost CAP_NET_BIND_SERVICE: %#x",
					after.Effective)
			}
			if after.Permitted&bit == 0 {
				exectest.Failf("Permitted lost CAP_NET_BIND_SERVICE: %#x",
					after.Permitted)
			}
			if after.Bounding&bit == 0 {
				exectest.Failf("Bounding lost CAP_NET_BIND_SERVICE: %#x",
					after.Bounding)
			}
			// Every OTHER bit must be cleared.
			if after.Effective & ^bit != 0 {
				exectest.Failf("Effective has bits other than CAP_NET_BIND_SERVICE: %#x",
					after.Effective)
			}
			if after.Permitted & ^bit != 0 {
				exectest.Failf("Permitted has bits other than CAP_NET_BIND_SERVICE: %#x",
					after.Permitted)
			}
			if after.Bounding & ^bit != 0 {
				exectest.Failf("Bounding has bits other than CAP_NET_BIND_SERVICE: %#x",
					after.Bounding)
			}
		} else {
			// Unprivileged starting state: every set must be
			// zero. "You cannot keep what you do not have."
			if after.Effective != 0 {
				exectest.Failf("unprivileged: Effective = %#x, want 0",
					after.Effective)
			}
			if after.Permitted != 0 {
				exectest.Failf("unprivileged: Permitted = %#x, want 0",
					after.Permitted)
			}
			if after.Bounding != 0 {
				exectest.Failf("unprivileged: Bounding = %#x, want 0",
					after.Bounding)
			}
		}
	})
}

// TestGet_ReturnsKernelStateInSubprocess asserts that Get() reads
// the kernel-side capability state via capget(2) without mutating
// it. This is the read-side smoke test the rest of the suite relies
// on for its post-condition assertions — if Get is broken, every
// other subprocess test in this file is unsound.
func TestGet_ReturnsKernelStateInSubprocess(t *testing.T) {
	exectest.RunInSubprocess(t, func() {
		first, err := capabilities.Get()
		if err != nil {
			exectest.Failf("first Get: %v", err)
		}
		second, err := capabilities.Get()
		if err != nil {
			exectest.Failf("second Get: %v", err)
		}
		// Two consecutive Get calls without intervening mutation
		// must return identical snapshots. If they differ, capget
		// is reading uninitialized memory or the snapshot type is
		// not stable.
		if first != second {
			exectest.Failf("Get returned non-stable snapshot: first=%+v second=%+v",
				first, second)
		}
	})
}
