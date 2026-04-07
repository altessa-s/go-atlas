// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package landlock

import (
	"errors"
	"fmt"
	"path/filepath"
)

// Sentinel errors for programmatic inspection via [errors.Is]. Errors
// returned from [Apply] also preserve the underlying [syscall.Errno] (when
// applicable) via multi-wrap, so callers can additionally use
// [errors.As] to recover the raw kernel error for branching logic.
var (
	// ErrUnsupported is returned when the running kernel does not support
	// Landlock at all (non-Linux platforms, or Linux < 5.13).
	ErrUnsupported = errors.New("landlock: kernel does not support Landlock (need Linux 5.13+)")

	// ErrFailed wraps any error from the underlying syscalls
	// (landlock_create_ruleset, landlock_add_rule, landlock_restrict_self,
	// or openat with O_PATH). The wrap chain preserves the kernel errno
	// so callers can match on syscall.Errno via [errors.As] in addition
	// to matching the sentinel via [errors.Is].
	ErrFailed = errors.New("landlock: ruleset setup failed")

	// ErrInvalidOption is returned from [Apply] when one of the supplied
	// option values is malformed (empty or non-absolute path). It is
	// distinct from [ErrFailed] so callers can separate "operator
	// misconfiguration" from "kernel rejected a well-formed ruleset".
	ErrInvalidOption = errors.New("landlock: invalid option")
)

// Apply installs a Landlock ruleset built from the provided options on the
// calling process. Apply is irreversible: once it returns nil, the process
// can never grant itself access outside the ruleset for the rest of its
// lifetime.
//
// On Linux, Apply sets PR_SET_NO_NEW_PRIVS on the calling process as its
// first step — this is a kernel prerequisite for landlock_restrict_self(2)
// on any process without CAP_SYS_ADMIN, and is itself irreversible.
// Invoking Apply therefore commits the process to NO_NEW_PRIVS as well as
// the filesystem ruleset. If the kernel is missing Landlock support the
// error is wrapped in [ErrUnsupported]; every other syscall-side failure
// is wrapped in [ErrFailed] and also exposes the underlying
// [syscall.Errno] via [errors.As]. A malformed option (empty or
// non-absolute path) is returned wrapped in [ErrInvalidOption] so callers
// can distinguish operator misconfiguration from kernel-side failures.
//
// Apply is intended to be called once per process lifetime, typically
// during startup. Concurrent invocations are not an API error — the
// kernel AND-composes overlapping rulesets — but the resulting composite
// allowlist is the intersection of every concurrent call, which is
// rarely what the caller wants. Serialize Apply via [sync.Once] or a
// deterministic startup sequence if multiple code paths may reach it.
//
// Example:
//
//	err := landlock.Apply(
//	    landlock.WithReadPaths("/etc", "/lib64", "/usr/lib64"),
//	    landlock.WithReadWritePaths("/var/log/myservice"),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
func Apply(opts ...Option) error {
	o := newOptions(opts...)
	if err := validateOptions(o); err != nil {
		return err
	}
	// Canonicalize paths after validation so that error messages from the
	// underlying syscalls reference the same representation the kernel
	// sees. Validation runs first so the "is empty" / "is not absolute"
	// errors still quote the caller's original input.
	cleanPaths(o.readPaths)
	cleanPaths(o.readWritePaths)
	return apply(o.readPaths, o.readWritePaths)
}

// validateOptions rejects empty and non-absolute path entries. It is
// invoked by [Apply] before any platform dispatch and by the internal
// tests; callers should not need to invoke it directly.
func validateOptions(o *options) error {
	if err := validatePaths("read paths", o.readPaths); err != nil {
		return err
	}
	if err := validatePaths("read-write paths", o.readWritePaths); err != nil {
		return err
	}
	return nil
}

func validatePaths(field string, paths []string) error {
	for i, p := range paths {
		if p == "" {
			return fmt.Errorf("%w: %s[%d] is empty", ErrInvalidOption, field, i)
		}
		if !filepath.IsAbs(p) {
			return fmt.Errorf("%w: %s[%d] %q is not an absolute path", ErrInvalidOption, field, i, p)
		}
	}
	return nil
}

// cleanPaths rewrites each entry with [filepath.Clean] in place. Callers
// must validate first — Clean("") returns ".", which would mask an empty
// path as a non-absolute one downstream.
func cleanPaths(paths []string) {
	for i, p := range paths {
		paths[i] = filepath.Clean(p)
	}
}
