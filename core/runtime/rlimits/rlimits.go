// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package rlimits

import (
	"errors"
	"fmt"
)

// Sentinel errors for programmatic inspection via [errors.Is]. Errors
// returned from [Apply] also preserve the underlying [syscall.Errno]
// (when applicable) via multi-wrap, so callers can additionally use
// [errors.As] to recover the raw kernel error for branching logic.
var (
	// ErrUnsupported is returned when the running platform does not
	// support setrlimit (non-Linux platforms).
	ErrUnsupported = errors.New("rlimits: platform does not support rlimits")

	// ErrFailed wraps any error from the underlying setrlimit(2)
	// syscall. The wrap chain preserves the kernel errno so callers can
	// match on [syscall.Errno] via [errors.As] in addition to matching
	// the sentinel via [errors.Is].
	ErrFailed = errors.New("rlimits: setrlimit failed")

	// ErrInvalidOption is returned from [Apply] when one of the supplied
	// option values is malformed (negative limit value). It is distinct
	// from [ErrFailed] so callers can separate "operator
	// misconfiguration" from "kernel rejected a well-formed rlimit".
	ErrInvalidOption = errors.New("rlimits: invalid option")
)

// Apply installs every configured rlimit on the calling process. Each
// option pins both soft and hard limits to the configured value, so the
// soft limit can never be raised back above it for the rest of the
// process's lifetime. Zero values leave the kernel default in place —
// except for [WithDisableCoreDumps], which intentionally pins
// RLIMIT_CORE to zero.
//
// Lowering a hard rlimit is irreversible for an unprivileged process.
// Apply is intended for one-shot startup-time hardening.
//
// Apply validates every option before invoking any syscall. A validation
// failure is wrapped in [ErrInvalidOption] so callers can distinguish
// bad configuration from kernel-side failures.
//
// Apply is safe to call from any goroutine but is typically invoked once
// during startup. Concurrent invocations that set overlapping rlimits
// compose as "the most recent Setrlimit wins" per resource — rarely
// what the caller wants. Serialize Apply via a deterministic startup
// sequence if multiple code paths may reach it.
func Apply(opts ...Option) error {
	o := newOptions(opts...)
	if err := validateOptions(o); err != nil {
		return err
	}
	return apply(o)
}

// validateOptions rejects negative rlimit values. It is invoked by
// [Apply] before any platform dispatch and by the internal tests;
// callers should not need to invoke it directly.
func validateOptions(o *options) error {
	if err := checkNonNegative("memoryBytes", o.memoryBytes); err != nil {
		return err
	}
	if err := checkNonNegative("maxOpenFiles", o.maxOpenFiles); err != nil {
		return err
	}
	if err := checkNonNegative("maxProcesses", o.maxProcesses); err != nil {
		return err
	}
	if err := checkNonNegative("maxFileSizeBytes", o.maxFileSizeBytes); err != nil {
		return err
	}
	return nil
}

func checkNonNegative(field string, v int64) error {
	if v < 0 {
		return fmt.Errorf("%w: %s=%d is negative", ErrInvalidOption, field, v)
	}
	return nil
}
