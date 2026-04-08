// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build linux

package plugins

import (
	"github.com/altessa-s/go-atlas/core/runtime/capabilities"
	"github.com/altessa-s/go-atlas/core/runtime/nonewprivs"
	"github.com/altessa-s/go-atlas/core/runtime/rlimits"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// applySandbox installs Linux process-hardening primitives configured by o.
// It is invoked exactly once by [Manager.ensureSandbox] before any .so file
// is opened. Every primitive applied here is irreversible for the lifetime
// of the host process.
//
// The sequence is always:
//  1. PR_SET_NO_NEW_PRIVS (via [nonewprivs.Set]) — defeats SUID escalation.
//  2. rlimits (via [rlimits.Apply]) — caps memory, FDs, procs, file size,
//     and optionally disables core dumps.
//  3. capabilities (via [capabilities.DropAll] / [capabilities.DropAllExcept]) —
//     drops Linux capabilities so plugins cannot inherit privileged syscalls
//     from the host process.
//  4. Landlock (via [applyLandlock]) — strict filesystem allowlist.
//
// The ordering matters:
//   - landlock_restrict_self requires NO_NEW_PRIVS;
//   - rlimits must run before capabilities so dropping CAP_SYS_RESOURCE
//     cannot break the setrlimit sequence;
//   - capabilities must run before Landlock because Landlock is the final
//     irreversible step and we want every other hardening pass complete
//     by the time it runs.
//
// When any individual step fails the function returns immediately,
// wrapping the underlying primitive's error in [ErrSandboxFailed] —
// partial application is intentional: a half-applied sandbox is better
// than silently dropping requested restrictions.
//
// Each primitive is a thin adapter over a reusable package under
// `core/runtime/`. The plugin sandbox owns the ordering and the mapping
// from [SandboxOptions] fields to primitive options; it adds no
// syscall-level logic of its own.
func applySandbox(o SandboxOptions) error {
	if !o.Enabled {
		return nil
	}

	if o.NoNewPrivs {
		if err := nonewprivs.Set(); err != nil {
			return coreerrs.JoinWrap(ErrSandboxFailed, err)
		}
	}

	rlimitOpts := []rlimits.Option{
		rlimits.WithMemoryBytes(o.MemoryLimitBytes),
		rlimits.WithMaxOpenFiles(o.MaxOpenFiles),
		rlimits.WithMaxProcesses(o.MaxProcesses),
		rlimits.WithMaxFileSizeBytes(o.MaxFileSizeBytes),
	}
	if o.DisableCoreDumps {
		rlimitOpts = append(rlimitOpts, rlimits.WithDisableCoreDumps())
	}
	if err := rlimits.Apply(rlimitOpts...); err != nil {
		return coreerrs.JoinWrap(ErrSandboxFailed, err)
	}

	if err := applyCapabilities(o.Capabilities); err != nil {
		return err
	}

	// Landlock is the final and irreversible step. It must run AFTER
	// PR_SET_NO_NEW_PRIVS because landlock_restrict_self requires either
	// NO_NEW_PRIVS or CAP_SYS_ADMIN. A no-op when not enabled.
	if err := applyLandlock(o.Landlock); err != nil {
		return err
	}

	return nil
}

// applyCapabilities is the plugin sandbox adapter over
// [capabilities.DropAll] / [capabilities.DropAllExcept]. It resolves
// operator-supplied capability names via [capabilities.ParseName] and
// wraps every error in [ErrSandboxFailed] so callers can still match
// the plugin-level sentinel via [errors.Is]. Invalid names are caught
// up-front by [config.PluginsCapabilities.Validate] at config-load
// time; reaching this function with an unknown name is a programming
// error that still surfaces as [ErrSandboxFailed].
func applyCapabilities(o CapabilitiesOptions) error {
	if !o.Enabled {
		return nil
	}
	if len(o.Keep) == 0 {
		if err := capabilities.DropAll(); err != nil {
			return coreerrs.JoinWrap(ErrSandboxFailed, err)
		}
		return nil
	}
	keep := make([]capabilities.Cap, 0, len(o.Keep))
	for _, name := range o.Keep {
		c, err := capabilities.ParseName(name)
		if err != nil {
			return coreerrs.JoinWrap(ErrSandboxFailed, err)
		}
		keep = append(keep, c)
	}
	if err := capabilities.DropAllExcept(keep...); err != nil {
		return coreerrs.JoinWrap(ErrSandboxFailed, err)
	}
	return nil
}
