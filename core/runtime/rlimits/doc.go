// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package rlimits installs Linux process resource limits (setrlimit(2))
// on the calling process. Each option caps both the soft and hard limit
// to the configured value, so the soft limit can never be raised back
// above it for the rest of the process's lifetime.
//
// Lowering a hard rlimit is irreversible for an unprivileged process.
// This package is intended for one-shot startup-time hardening — call
// [Apply] early in main, before any long-running work begins.
//
// # Quick start
//
// Cap the process at 512 MiB of virtual address space and 4 096 open
// file descriptors, and disable core dumps:
//
//	err := rlimits.Apply(
//	    rlimits.WithMemoryBytes(512 << 20),
//	    rlimits.WithMaxOpenFiles(4096),
//	    rlimits.WithDisableCoreDumps(),
//	)
//	if err != nil {
//	    log.Fatalf("rlimits: %v", err)
//	}
//
// # Scope and caveats
//
//   - Linux only. On non-Linux platforms [Apply] returns [ErrUnsupported]
//     so cross-platform code can import the package without build tags.
//   - Process-wide. Every goroutine, every child process, every
//     in-process database driver shares the same resource pool. Tune
//     MaxOpenFiles with care — it counts sockets, files, pipes, epoll,
//     timerfd, signalfd, etc.
//   - Irreversible for hard-limit reductions. Once a hard limit is
//     lowered, an unprivileged process cannot raise it.
//   - Zero = unset. A zero value leaves the kernel default in place for
//     that resource, except for [WithDisableCoreDumps] which intentionally
//     pins RLIMIT_CORE to zero.
//   - Defense-in-depth, not isolation. A compromised dependency already
//     loaded in-process can still allocate up to the configured cap and
//     read any file the process already holds open.
//
// # Resources
//
// The following rlimits are supported (every option is a no-op when
// unset):
//
//	RLIMIT_AS     via WithMemoryBytes      — virtual address space cap
//	RLIMIT_NOFILE via WithMaxOpenFiles     — file descriptor cap
//	RLIMIT_NPROC  via WithMaxProcesses     — process cap (per real UID)
//	RLIMIT_FSIZE  via WithMaxFileSizeBytes — max file size the process can write
//	RLIMIT_CORE   via WithDisableCoreDumps — core dump size (pinned to 0)
package rlimits
