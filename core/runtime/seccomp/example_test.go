// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package seccomp_test

import (
	"errors"
	"log"

	"github.com/altessa-s/go-atlas/core/runtime/seccomp"
)

// ExampleBlockDangerousSyscalls demonstrates the canonical startup
// usage of the denylist primitive. Not executed (no "Output:"
// directive) because installing a seccomp filter is irreversible and
// would poison every subsequent test in the binary; instead, the
// example is compile-checked so that any future change to the
// BlockDangerousSyscalls signature breaks this file and forces a
// review.
func ExampleBlockDangerousSyscalls() {
	err := seccomp.BlockDangerousSyscalls()
	switch {
	case err == nil:
		// The process can no longer call mount, kexec, init_module,
		// reboot, ptrace, bpf, userfaultfd, and ~15 other syscalls.
		// Every denied syscall returns EPERM.
	case errors.Is(err, seccomp.ErrUnsupported):
		// Non-Linux platform, or Linux on an exotic arch. Fail open
		// so cross-platform builds still work.
		log.Print("seccomp: platform does not support seccomp-BPF; continuing without filter")
	case errors.Is(err, seccomp.ErrFailed):
		log.Fatalf("seccomp: kernel rejected filter: %v", err)
	default:
		log.Fatalf("seccomp: unexpected error: %v", err)
	}
}
