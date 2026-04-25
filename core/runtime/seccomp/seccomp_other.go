// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build !linux || (linux && !amd64 && !arm64)

package seccomp

// install is the stub for non-Linux platforms and Linux on
// architectures other than amd64 and arm64. The seccomp-BPF filter
// uses architecture-specific syscall numbers and an AUDIT_ARCH_*
// token in the arch-prologue; supporting a new architecture requires
// a per-arch constants file plus verification that every syscall in
// [dangerousSyscalls] is present in golang.org/x/sys/unix for that
// arch. Callers on unsupported platforms receive [ErrUnsupported]
// so cross-platform code fails open with a clear error instead of
// silently installing no filter.
func install() error {
	return ErrUnsupported
}
