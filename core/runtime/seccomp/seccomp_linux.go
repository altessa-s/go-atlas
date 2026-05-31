// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build linux && (amd64 || arm64)

package seccomp

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/altessa-s/go-atlas/core/runtime/nonewprivs"

	"golang.org/x/sys/unix"
)

// dangerousSyscalls is the fixed denylist installed by
// [BlockDangerousSyscalls]. Every entry is a syscall the Go runtime
// never calls and which a legitimate plugin host has no reason to
// invoke either. The numbers are resolved from [golang.org/x/sys/unix]
// at compile time so the list is automatically correct on both
// linux/amd64 and linux/arm64.
//
// DO NOT add "maybe useful" syscalls here — every addition risks
// breaking a future Go version that starts calling the syscall from
// its runtime. The bar for a new entry is: the kernel has never
// legitimately needed this syscall from user code, and no production
// Go program has ever called it through its standard library.
//
// Kept sorted by syscall number within each category for stable
// review diffs.
var dangerousSyscalls = []uint32{
	// Filesystem manipulation.
	uint32(unix.SYS_MOUNT),
	uint32(unix.SYS_UMOUNT2),
	uint32(unix.SYS_PIVOT_ROOT),
	uint32(unix.SYS_CHROOT),
	uint32(unix.SYS_SWAPON),
	uint32(unix.SYS_SWAPOFF),

	// Kernel module loading.
	uint32(unix.SYS_INIT_MODULE),
	uint32(unix.SYS_FINIT_MODULE),
	uint32(unix.SYS_DELETE_MODULE),

	// Kernel reload.
	uint32(unix.SYS_KEXEC_FILE_LOAD),

	// System control.
	uint32(unix.SYS_REBOOT),

	// Debugging / memory inspection.
	uint32(unix.SYS_PTRACE),
	uint32(unix.SYS_PROCESS_VM_READV),
	uint32(unix.SYS_PROCESS_VM_WRITEV),

	// Namespace manipulation.
	uint32(unix.SYS_UNSHARE),
	uint32(unix.SYS_SETNS),

	// Keyring.
	uint32(unix.SYS_KEYCTL),
	uint32(unix.SYS_ADD_KEY),
	uint32(unix.SYS_REQUEST_KEY),

	// Exotic escalation vectors.
	uint32(unix.SYS_USERFAULTFD),
	uint32(unix.SYS_PERF_EVENT_OPEN),
	uint32(unix.SYS_BPF),
}

// BPF return actions. These are 32-bit masks returned by the filter
// and interpreted by the kernel.
//
//	retAllow: let the syscall through unchanged.
//	retDeny:  return EPERM to the caller without invoking the syscall.
//	retKill:  send SIGKILL to every thread in the thread group.
const (
	retAllow = uint32(unix.SECCOMP_RET_ALLOW)
	retDeny  = uint32(unix.SECCOMP_RET_ERRNO) | uint32(unix.EPERM)
	retKill  = uint32(unix.SECCOMP_RET_KILL_PROCESS)
)

// Offsets into the seccomp_data struct. Matches Linux uapi:
//
//	struct seccomp_data {
//	    int nr;                 // syscall number (offset 0)
//	    __u32 arch;             // AUDIT_ARCH_* (offset 4)
//	    __u64 instruction_pointer;
//	    __u64 args[6];
//	};
const (
	seccompDataNrOffset   = 0
	seccompDataArchOffset = 4

	// bpfArchToKillOffset is the BPF jump distance from the arch-check
	// instruction (pos 1) to the KILL action (pos N+5): (N+5) − 2 = N+3.
	bpfArchToKillOffset = 3
)

// buildFilter constructs the classic BPF program that implements the
// denylist. The program layout is documented in the package plan:
//
//	pos 0          LD [arch]           # load seccomp_data.arch
//	pos 1          JEQ expectedArch    # if mismatch, goto KILL
//	pos 2          LD [nr]             # load seccomp_data.nr
//	pos 3..2+N     JEQ SYS_i           # N jumps; on match goto DENY
//	pos 3+N        RET ALLOW           # default: allow
//	pos 4+N        RET ERRNO(EPERM)    # DENY target
//	pos 5+N        RET KILL_PROCESS    # KILL target (arch mismatch)
//
// The function is pure — no syscalls, no globals — so tests can
// exercise it directly on any platform to verify the instruction
// layout and offset arithmetic.
func buildFilter() []unix.SockFilter {
	n := uint8(len(dangerousSyscalls))
	prog := make([]unix.SockFilter, 0, int(n)+6)

	// pos 0: LD [arch]
	prog = append(prog, unix.SockFilter{
		Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS,
		K:    seccompDataArchOffset,
	})

	// pos 1: JEQ expectedArch; fallthrough on match, jump to KILL
	// on mismatch. KILL is at position N+5, so jf = (N+5) - 2 = N+3.
	prog = append(prog, unix.SockFilter{
		Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K,
		Jt:   0,
		Jf:   n + bpfArchToKillOffset,
		K:    expectedArch,
	})

	// pos 2: LD [nr]
	prog = append(prog, unix.SockFilter{
		Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS,
		K:    seccompDataNrOffset,
	})

	// pos 3..2+N: JEQ SYS_i; on match jump to DENY at position N+4.
	// For the i-th JEQ (1-indexed) at position 2+i, jt = N + 1 - i so
	// that (2+i) + 1 + jt = 2+i + 1 + (N+1-i) = N+4 = DENY.
	for i, nr := range dangerousSyscalls {
		prog = append(prog, unix.SockFilter{
			Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K,
			Jt:   n - uint8(i),
			Jf:   0,
			K:    nr,
		})
	}

	// pos 3+N: RET ALLOW
	prog = append(prog, unix.SockFilter{
		Code: unix.BPF_RET | unix.BPF_K,
		K:    retAllow,
	})

	// pos 4+N: RET ERRNO(EPERM)
	prog = append(prog, unix.SockFilter{
		Code: unix.BPF_RET | unix.BPF_K,
		K:    retDeny,
	})

	// pos 5+N: RET KILL_PROCESS
	prog = append(prog, unix.SockFilter{
		Code: unix.BPF_RET | unix.BPF_K,
		K:    retKill,
	})

	return prog
}

// install is the Linux implementation of [BlockDangerousSyscalls]. It:
//
//  1. Sets PR_SET_NO_NEW_PRIVS via [nonewprivs.Set]. This is a kernel
//     prerequisite for seccomp(2) on any process without
//     CAP_SYS_ADMIN.
//  2. Builds the denylist BPF program via [buildFilter].
//  3. Installs the program via seccomp(SECCOMP_SET_MODE_FILTER,
//     SECCOMP_FILTER_FLAG_TSYNC, &prog).
//
// TSYNC is non-negotiable: without it the filter would apply only to
// the calling goroutine's current OS thread, leaving every other
// thread of the Go runtime unprotected. If any peer thread cannot be
// synced, the seccomp syscall returns the failing TID as a positive
// integer (not an errno), and install reports that as ErrFailed with
// a clear message.
//
// install pins the goroutine to its OS thread via [runtime.LockOSThread]
// for the full sequence. Both PR_SET_NO_NEW_PRIVS and seccomp(2) are
// per-thread operations, and without pinning the Go scheduler may
// migrate the goroutine between the prctl and the seccomp call — the
// seccomp syscall would then land on a thread that does not have
// NO_NEW_PRIVS set and fail with EACCES (since the process lacks
// CAP_SYS_ADMIN). TSYNC propagates the installed filter to every peer
// thread, so releasing the pin after install is safe.
func install() (retErr error) {
	// Pin to the current OS thread for the duration of the install
	// sequence. nonewprivs.Set and seccomp(SET_MODE_FILTER) must execute
	// on the same thread or the kernel rejects the filter.
	runtime.LockOSThread()
	defer func() {
		// On success the filter is TSYNC'd to every thread, so the
		// pin is no longer needed and we release it. On failure we
		// intentionally leave the thread locked and let the goroutine
		// exit carry the lock away — we cannot safely reuse a thread
		// that may have been left with NO_NEW_PRIVS set while the
		// seccomp filter install failed.
		if retErr == nil {
			runtime.UnlockOSThread()
		}
	}()

	if err := nonewprivs.Set(); err != nil {
		return fmt.Errorf("%w: %w", ErrFailed, err)
	}

	prog := buildFilter()
	fprog := unix.SockFprog{
		Len:    uint16(len(prog)),
		Filter: &prog[0],
	}

	r1, _, errno := unix.Syscall(
		unix.SYS_SECCOMP,
		uintptr(unix.SECCOMP_SET_MODE_FILTER),
		uintptr(unix.SECCOMP_FILTER_FLAG_TSYNC),
		uintptr(unsafe.Pointer(&fprog)), // #nosec G103 -- required by seccomp(2) ABI; fprog lives on the stack for the syscall duration
	)
	if errno != 0 {
		return fmt.Errorf("%w: seccomp(SET_MODE_FILTER): %w", ErrFailed, errno)
	}
	if r1 != 0 {
		// TSYNC failure: the kernel returns the TID of the peer
		// thread that could not be synchronized. No errno is set.
		return fmt.Errorf(
			"%w: seccomp(SET_MODE_FILTER) TSYNC failed on thread %d",
			ErrFailed, r1,
		)
	}
	return nil
}
