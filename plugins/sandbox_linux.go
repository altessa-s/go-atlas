// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build linux

package plugins

import (
	"fmt"
	"strings"

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
//   - landlock_restrict_self requires NO_NEW_PRIVS, so [nonewprivs.Set]
//     runs first.
//   - rlimits run before capabilities. The stated reason in the
//     primitive's docs is "do the cheapest, most-recoverable pass
//     first": setrlimit is per-process and bounded to lowering the
//     soft limit (which any unprivileged process can do, regardless
//     of CAP_SYS_RESOURCE — the kernel only requires that capability
//     for raising hard limits or setting them above the kernel cap),
//     while capabilities mutates per-thread state irreversibly. Doing
//     the side-effect-light pass first means a misconfigured rlimit
//     (rejected by validation) does not leave behind a half-applied
//     capability state.
//   - capabilities run before Landlock because Landlock is the final
//     irreversible step and we want every other hardening pass
//     complete by the time it runs.
//
// On failure, applySandbox returns an error wrapped in [ErrSandboxFailed]
// AND annotated with which step failed and what state the process is
// already in. The annotation is critical because partial application
// is intentional — a half-applied sandbox is better than silently
// dropping requested restrictions, but the operator must be able to
// reconstruct the partial state from the error to decide whether to
// terminate or continue degraded.
//
// Each primitive is a thin adapter over a reusable package under
// `core/runtime/`. The plugin sandbox owns the ordering and the mapping
// from [SandboxOptions] fields to primitive options; it adds no
// syscall-level logic of its own.
func applySandbox(o SandboxOptions) error {
	if !o.Enabled {
		return nil
	}

	// Track which steps have committed so the partial-state error
	// message can tell the operator exactly which primitives are
	// already in effect and which were not reached.
	var applied []string

	if o.NoNewPrivs {
		if err := nonewprivs.Set(); err != nil {
			return wrapSandboxStepErr("nonewprivs.Set", applied, err)
		}
		applied = append(applied, "nonewprivs")
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
		return wrapSandboxStepErr("rlimits.Apply", applied, err)
	}
	applied = append(applied, "rlimits")

	if err := applyCapabilities(o.Capabilities); err != nil {
		return wrapSandboxStepErr("capabilities", applied, err)
	}
	if o.Capabilities.Enabled {
		applied = append(applied, "capabilities")
	}

	// Landlock is the final and irreversible step. It must run AFTER
	// PR_SET_NO_NEW_PRIVS because landlock_restrict_self requires either
	// NO_NEW_PRIVS or CAP_SYS_ADMIN. A no-op when not enabled.
	if err := applyLandlock(o.Landlock); err != nil {
		return wrapSandboxStepErr("landlock", applied, err)
	}

	return nil
}

// wrapSandboxStepErr produces the structured error returned by
// applySandbox on a step failure. The message lists every primitive
// that has already committed to the host process and identifies the
// failing step. Operators can match programmatically on
// [ErrSandboxFailed] via [errors.Is], read the step / applied list
// from the message, and recover the underlying primitive errno via
// [errors.As].
func wrapSandboxStepErr(step string, applied []string, cause error) error {
	appliedDesc := "none"
	if len(applied) > 0 {
		appliedDesc = strings.Join(applied, ",")
	}
	annotated := fmt.Errorf(
		"step=%s applied=[%s] (the host process already has the listed primitives in effect; "+
			"a half-applied sandbox is intentional — see [Manager.ensureSandbox] doc): %w",
		step, appliedDesc, cause,
	)
	return coreerrs.JoinWrap(ErrSandboxFailed, annotated)
}

// applyCapabilities is the plugin sandbox adapter over
// [capabilities.DropAll] / [capabilities.DropAllExcept]. It resolves
// operator-supplied capability names via [capabilities.ParseName] and
// returns the underlying primitive error unwrapped — the caller
// ([applySandbox]) is responsible for wrapping under [ErrSandboxFailed]
// with the partial-state context. Invalid names are caught up-front
// by [config.PluginsCapabilities.Validate] at config-load time;
// reaching this function with an unknown name is a programming error.
func applyCapabilities(o CapabilitiesOptions) error {
	if !o.Enabled {
		return nil
	}
	if len(o.Keep) == 0 {
		return capabilities.DropAll()
	}
	keep := make([]capabilities.Cap, 0, len(o.Keep))
	for _, name := range o.Keep {
		c, err := capabilities.ParseName(name)
		if err != nil {
			return err
		}
		keep = append(keep, c)
	}
	return capabilities.DropAllExcept(keep...)
}
