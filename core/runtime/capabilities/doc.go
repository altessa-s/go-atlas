// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package capabilities wraps the Linux capabilities subsystem
// (capset(2), capget(2), prctl(2) PR_CAPBSET_*, PR_CAP_AMBIENT_*). It
// lets a process drop capabilities at startup and confine itself to a
// strict allowlist without requiring cgo or libcap.
//
// # Quick start
//
// Drop every capability before loading untrusted code:
//
//	if err := capabilities.DropAll(); err != nil {
//	    log.Fatalf("capabilities: %v", err)
//	}
//
// Preserve one capability (typical "bind a privileged port then drop
// everything else" pattern):
//
//	if err := capabilities.DropAllExcept(capabilities.CAP_NET_BIND_SERVICE); err != nil {
//	    log.Fatalf("capabilities: %v", err)
//	}
//
// # Four-set model
//
// Linux capabilities are not a single mask but five related sets
// tracked per thread:
//
//   - Effective   — the bits currently in force for privileged syscalls.
//   - Permitted   — the ceiling from which effective can be raised.
//   - Inheritable — passed to children across execve; rarely needed.
//   - Bounding    — the ceiling from which permitted can be raised; a
//     bit dropped from bounding can never be re-acquired.
//   - Ambient     — preserved across a non-privileged execve when also
//     present in permitted and inheritable.
//
// [Sets] reports all five as 64-bit masks. [Apply] accepts a full
// snapshot and installs it atomically (where the kernel permits);
// [DropAll] / [DropAllExcept] are thin wrappers implementing the two
// most common startup patterns.
//
// # Threat model
//
// This package is a defense-in-depth primitive. It restricts the
// capability set visible to subsequent syscalls. It does NOT:
//
//   - Sandbox filesystem access (use
//     [github.com/altessa-s/go-atlas/core/runtime/landlock]).
//   - Cap resource usage (use
//     [github.com/altessa-s/go-atlas/core/runtime/rlimits]).
//   - Defeat SUID escalation on exec (use
//     [github.com/altessa-s/go-atlas/core/runtime/nonewprivs]).
//   - Protect against a malicious binary already loaded in-process —
//     such a binary still has full access to host memory.
//
// Combine this package with the others for a layered startup hardening
// sequence: nonewprivs → rlimits → capabilities → seccomp → landlock.
//
// # Caveats
//
//   - Linux only. On non-Linux platforms every function returns
//     [ErrUnsupported] or its zero-value equivalent so cross-platform
//     code can import the package without build tags.
//   - Per-thread scope. capset(2) mutates only the calling thread. The
//     package pins the goroutine to its OS thread around every
//     mutating call, but for the change to cover the whole process
//     callers must apply before spawning any goroutines.
//   - Irreversible ceilings. Bits dropped from the bounding set cannot
//     be raised by any descendant, and the inheritable/ambient
//     invariants are enforced by the kernel — not by this package.
//   - Raising bits is possible only within the current ceilings.
//     Dropping is always possible for an unprivileged process.
package capabilities
