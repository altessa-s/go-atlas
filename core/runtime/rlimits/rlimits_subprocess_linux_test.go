// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build linux

// Subprocess tests for the irreversible Linux primitives in this
// package. Lowering an rlimit pins the soft+hard limits and an
// unprivileged process cannot raise them again, so a successful
// Apply call would contaminate every later test in the same binary.
// These tests run in a re-execed child that exits immediately after
// the assertion.

package rlimits_test

import (
	"syscall"
	"testing"

	"github.com/altessa-s/go-atlas/core/runtime/internal/exectest"
	"github.com/altessa-s/go-atlas/core/runtime/rlimits"
)

// TestApply_NoFileTakesEffectInSubprocess pins RLIMIT_NOFILE to a
// small value and then verifies via a fresh getrlimit(2) call that
// both soft and hard limits report exactly that value. The Apply
// contract pins Cur=Max=value; if a future refactor diverged from
// that, this test catches it.
func TestApply_NoFileTakesEffectInSubprocess(t *testing.T) {
	exectest.RunInSubprocess(t, func() {
		const want = 256
		if err := rlimits.Apply(rlimits.WithMaxOpenFiles(want)); err != nil {
			exectest.Failf("Apply(WithMaxOpenFiles(%d)): %v", want, err)
		}

		var got syscall.Rlimit
		if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &got); err != nil {
			exectest.Failf("Getrlimit(RLIMIT_NOFILE): %v", err)
		}
		if got.Cur != want {
			exectest.Failf("RLIMIT_NOFILE.Cur = %d, want %d", got.Cur, want)
		}
		if got.Max != want {
			exectest.Failf("RLIMIT_NOFILE.Max = %d, want %d "+
				"(Apply must pin Cur=Max so the soft limit cannot be raised back)",
				got.Max, want)
		}
	})
}

// TestApply_DisableCoreDumpsTakesEffectInSubprocess verifies that
// WithDisableCoreDumps pins RLIMIT_CORE to zero. This is the only
// rlimit option where zero is a meaningful operator request rather
// than "leave alone".
func TestApply_DisableCoreDumpsTakesEffectInSubprocess(t *testing.T) {
	exectest.RunInSubprocess(t, func() {
		if err := rlimits.Apply(rlimits.WithDisableCoreDumps()); err != nil {
			exectest.Failf("Apply(WithDisableCoreDumps()): %v", err)
		}

		var got syscall.Rlimit
		if err := syscall.Getrlimit(syscall.RLIMIT_CORE, &got); err != nil {
			exectest.Failf("Getrlimit(RLIMIT_CORE): %v", err)
		}
		if got.Cur != 0 || got.Max != 0 {
			exectest.Failf("RLIMIT_CORE = (Cur=%d, Max=%d), want (0, 0)",
				got.Cur, got.Max)
		}
	})
}

// TestApply_MultipleLimitsCompose checks that a single Apply call
// with several options installs all of them. setrlimit is per-resource
// so they cannot interfere — but a copy-paste bug in the loop body
// inside the Linux dispatch is exactly the kind of regression a
// composite test catches that single-resource tests would miss.
func TestApply_MultipleLimitsCompose(t *testing.T) {
	exectest.RunInSubprocess(t, func() {
		const (
			wantNofile = 512
			wantFsize  = 1 << 20 // 1 MiB
		)
		err := rlimits.Apply(
			rlimits.WithMaxOpenFiles(wantNofile),
			rlimits.WithMaxFileSizeBytes(wantFsize),
			rlimits.WithDisableCoreDumps(),
		)
		if err != nil {
			exectest.Failf("Apply(composite): %v", err)
		}

		var nofile, fsize, core syscall.Rlimit
		if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &nofile); err != nil {
			exectest.Failf("Getrlimit(NOFILE): %v", err)
		}
		if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &fsize); err != nil {
			exectest.Failf("Getrlimit(FSIZE): %v", err)
		}
		if err := syscall.Getrlimit(syscall.RLIMIT_CORE, &core); err != nil {
			exectest.Failf("Getrlimit(CORE): %v", err)
		}
		if nofile.Cur != wantNofile {
			exectest.Failf("NOFILE.Cur = %d, want %d", nofile.Cur, wantNofile)
		}
		if fsize.Cur != wantFsize {
			exectest.Failf("FSIZE.Cur = %d, want %d", fsize.Cur, wantFsize)
		}
		if core.Cur != 0 {
			exectest.Failf("CORE.Cur = %d, want 0 (DisableCoreDumps not honored)",
				core.Cur)
		}
	})
}
