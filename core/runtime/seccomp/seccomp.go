// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package seccomp

import "errors"

// Sentinel errors for programmatic inspection via [errors.Is]. Errors
// returned from [BlockDangerousSyscalls] also preserve the underlying
// [syscall.Errno] via multi-wrap, so callers can additionally use
// [errors.As] to recover the raw kernel error for branching logic.
var (
	// ErrUnsupported is returned when the running platform does not
	// support this package: non-Linux platforms, or Linux on an
	// architecture other than amd64 or arm64. Callers that want to
	// fail-open on unsupported platforms should check for this
	// sentinel via [errors.Is].
	ErrUnsupported = errors.New("seccomp: platform does not support seccomp-BPF")

	// ErrFailed wraps any error from the underlying seccomp(2) or
	// prctl(2) syscalls, or from the [nonewprivs.Set] prerequisite.
	// The wrap chain preserves the kernel errno so callers can match
	// on [syscall.Errno] via [errors.As] in addition to matching the
	// sentinel via [errors.Is].
	ErrFailed = errors.New("seccomp: filter installation failed")
)

// BlockDangerousSyscalls installs a seccomp-BPF filter that denies
// a fixed list of syscalls a Go process never legitimately calls:
// mount, kexec_file_load, init_module, reboot, ptrace, bpf,
// userfaultfd, and ~15 others. See the package documentation for the
// full list and the rationale for why this primitive is a fixed
// denylist rather than a curated allowlist.
//
// On Linux, BlockDangerousSyscalls first sets PR_SET_NO_NEW_PRIVS
// (via [github.com/altessa-s/go-atlas/core/runtime/nonewprivs.Set])
// because seccomp(2) requires either that prctl or CAP_SYS_ADMIN.
// It then installs the filter with SECCOMP_FILTER_FLAG_TSYNC so the
// filter propagates to every thread in the thread group — critical
// for a Go process where goroutines run on multiple OS threads.
//
// On non-Linux platforms, or on Linux architectures other than
// amd64 and arm64, BlockDangerousSyscalls returns [ErrUnsupported]
// without touching any syscalls.
//
// The operation is irreversible. Seccomp filters form a stack and
// can only be made more restrictive; there is no "remove filter"
// syscall. BlockDangerousSyscalls is intended for one-shot
// startup-time hardening.
//
// Concurrent invocations are harmless but wasteful — the kernel
// simply stacks a second copy of the same filter. Typical callers
// invoke this once during startup, guarded by [sync.Once] or a
// deterministic startup sequence.
func BlockDangerousSyscalls() error {
	return install()
}
