// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package seccomp installs a Linux seccomp-BPF filter that denies a
// fixed list of syscalls a Go process never legitimately calls. It is
// a defense-in-depth primitive aimed at reducing the blast radius of a
// compromised plugin or dependency, NOT a complete syscall sandbox.
//
// # Denylist, not allowlist
//
// This package intentionally ships a **fixed denylist** of ~23
// syscalls (mount, kexec, init_module, reboot, ptrace, bpf,
// userfaultfd, and friends) and does NOT expose an API for curating
// an allowlist. A curated allowlist for a Go-runtime process is an
// anti-pattern:
//
//   - The Go runtime calls many syscalls that change between Go
//     versions and kernel versions (clone vs clone3, pidfd_open,
//     madvise, rseq, membarrier, getrandom, futex, epoll_pwait2, ...).
//   - An over-eager allowlist breaks the process under SIGSYS at
//     random moments in production, usually not at startup.
//   - A safe allowlist has to be generated from production traces
//     per Go version × kernel version, which is infrastructure work,
//     not a Go-package concern.
//
// For syscall-level filtering beyond this package's fixed denylist,
// configure seccomp at the container layer instead: Kubernetes
// securityContext.seccompProfile, Docker --security-opt seccomp=...,
// or systemd SystemCallFilter=.
//
// # Quick start
//
// Install the filter at startup, before loading any untrusted code:
//
//	if err := seccomp.BlockDangerousSyscalls(); err != nil {
//	    log.Fatalf("seccomp: %v", err)
//	}
//
// The filter applies to every thread in the thread group via the
// TSYNC flag, so it covers Go goroutines correctly regardless of
// which OS thread they run on.
//
// # What the denylist blocks
//
//	Filesystem manipulation:
//	  mount, umount2, pivot_root, chroot, swapon, swapoff
//
//	Kernel module loading:
//	  init_module, finit_module, delete_module
//
//	Kernel reload:
//	  kexec_load, kexec_file_load
//
//	System control:
//	  reboot
//
//	Debugging / inspection:
//	  ptrace, process_vm_readv, process_vm_writev
//
//	Namespace manipulation:
//	  unshare, setns
//
//	Keyring:
//	  keyctl, add_key, request_key
//
//	Exotic escalation vectors:
//	  userfaultfd, perf_event_open, bpf
//
// A blocked syscall returns EPERM to the caller. An architecture
// mismatch (e.g. an x32 syscall on an amd64 kernel) kills the
// process outright, because syscall numbers differ across arches and
// a filter that trusts the wrong numbering is worse than no filter.
//
// # Threat model
//
// This package restricts the ambient syscall set visible to the
// process after [BlockDangerousSyscalls] returns. It does NOT:
//
//   - Sandbox filesystem access (use
//     [github.com/altessa-s/go-atlas/core/runtime/landlock]).
//   - Cap resource usage (use
//     [github.com/altessa-s/go-atlas/core/runtime/rlimits]).
//   - Drop Linux capabilities (use
//     [github.com/altessa-s/go-atlas/core/runtime/capabilities]).
//   - Defeat SUID escalation on exec (use
//     [github.com/altessa-s/go-atlas/core/runtime/nonewprivs]).
//
// Combine this package with the other four for a layered hardening
// sequence: nonewprivs → rlimits → capabilities → seccomp → landlock.
//
// # Caveats
//
//   - Linux, amd64 or arm64 only. On non-Linux platforms and on
//     exotic Linux architectures (mips, ppc64, ...)
//     [BlockDangerousSyscalls] returns [ErrUnsupported]. Callers
//     that want to fail-open on unsupported platforms should check
//     for [ErrUnsupported] explicitly.
//   - Kernel 3.17+ for basic seccomp-BPF, 3.17+ for TSYNC. All
//     supported kernels meet this threshold.
//   - Irreversible. Seccomp filters form a stack and can only be
//     made more restrictive; there is no "remove filter" syscall.
//   - PR_SET_NO_NEW_PRIVS is a prerequisite on processes without
//     CAP_SYS_ADMIN. [BlockDangerousSyscalls] sets it
//     automatically (via [nonewprivs.Set]), so callers do not need
//     to invoke prctl themselves.
//   - Defense in depth, not isolation. A malicious dependency
//     already loaded in-process can still read every byte of memory
//     in the host's address space. Seccomp closes specific
//     escalation vectors (container escape, kernel-module loading,
//     ptrace, bpf, etc.), not the ambient code-execution vector.
package seccomp
