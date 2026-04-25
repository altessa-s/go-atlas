// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build linux && arm64

package seccomp

import "golang.org/x/sys/unix"

// expectedArch is the AUDIT_ARCH_* token the filter's arch-prologue
// compares against. On arm64 this is AUDIT_ARCH_AARCH64. A kernel
// that hands us seccomp_data.arch with any other value (e.g. a
// 32-bit arm binary running under multiarch dispatch) causes the
// filter to kill the process — syscall numbers differ by arch and
// trusting the wrong numbering is worse than no filter at all.
const expectedArch uint32 = unix.AUDIT_ARCH_AARCH64
