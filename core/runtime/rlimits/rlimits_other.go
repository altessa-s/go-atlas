// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build !linux

package rlimits

// apply is the non-Linux stub for the [Apply] platform dispatch.
// setrlimit(2) is a POSIX syscall — most Unix platforms in practice do
// support it — but this package currently ships only the Linux
// implementation because the rlimit resource set and semantics differ
// subtly across kernels, and we do not have test coverage on BSD or
// darwin. Callers on non-Linux platforms receive [ErrUnsupported] so
// cross-platform code fails open rather than silently dropping
// configured rlimits.
func apply(_ *options) error {
	return ErrUnsupported
}
