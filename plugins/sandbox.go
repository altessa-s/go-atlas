// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/core/runtime/capabilities"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
)

// SandboxOptions configures Linux process-hardening primitives applied lazily
// on the first [Manager.Load] when [Enabled] is true.
//
// IMPORTANT — read this before enabling:
//
//   - Linux only. On other platforms an enabled sandbox makes Manager.Load
//     fail with [ErrSandboxUnsupported].
//   - Applied lazily and irreversible. Host code that runs before the first
//     Load is unrestricted; from that point onward the limits apply to the
//     entire host process.
//   - The primitives are process-wide. Setting MaxOpenFiles too low starves
//     the host's database/HTTP/Redis pools, not just plugin code.
//   - This is defense-in-depth, not isolation. A malicious plugin still has
//     full access to host memory and can corrupt or exfiltrate anything in
//     the address space.
//
// Use [SandboxOptionsFromConfig] to populate this from [config.PluginsSandbox].
type SandboxOptions struct {
	// Enabled turns the sandbox on. When false the manager skips applySandbox
	// entirely and every other field is ignored.
	Enabled bool

	// NoNewPrivs sets PR_SET_NO_NEW_PRIVS so the process and any binary it
	// later execs cannot gain privileges via SUID/SGID. Note: enabling
	// [LandlockOptions.Enabled] also forces NO_NEW_PRIVS unconditionally
	// (it is a kernel prerequisite for landlock_restrict_self), so this
	// field is only meaningful when Landlock is disabled.
	NoNewPrivs bool

	// MemoryLimitBytes caps RLIMIT_AS. 0 = unset.
	MemoryLimitBytes int64

	// MaxOpenFiles caps RLIMIT_NOFILE. 0 = unset.
	MaxOpenFiles int64

	// MaxProcesses caps RLIMIT_NPROC. 0 = unset. NOTE: RLIMIT_NPROC is
	// per real UID on Linux, not per process — the kernel counts every
	// process owned by the same UID, including unrelated processes
	// running under that UID. On a host running multiple service
	// instances under one UID, or under a shared system user
	// (`nobody`, `daemon`), a low value can starve unrelated processes
	// and the symptom (`fork: Resource temporarily unavailable` from a
	// sibling process) is hard to trace back. Prefer systemd
	// TasksMax= or cgroup pids.max for per-instance bounds.
	MaxProcesses int64

	// MaxFileSizeBytes caps RLIMIT_FSIZE. 0 = unset.
	MaxFileSizeBytes int64

	// DisableCoreDumps sets RLIMIT_CORE = 0 to prevent leaking memory contents
	// (including secrets) via a post-crash core dump.
	DisableCoreDumps bool

	// Capabilities is the Linux capability-dropping pass applied after
	// rlimits and before Landlock. See [CapabilitiesOptions] for the
	// operator contract.
	Capabilities CapabilitiesOptions

	// Landlock is the strict filesystem allowlist applied via the Linux
	// Landlock LSM (kernel >= 5.13). See [LandlockOptions] for the operator
	// contract.
	Landlock LandlockOptions
}

// LandlockOptions configures a kernel-enforced filesystem allowlist applied
// after the rlimits when [SandboxOptions.Enabled] is true.
//
// Landlock is a strict allowlist: nothing is auto-added. Operators MUST list
// the plugin directory, the dynamic loader, libc, every shared library the
// plugin imports, and every host-side path the application needs to read,
// execute, or write. A missing entry surfaces as `permission denied` from
// `plugin.Open` or any subsequent host I/O — there is no clearer error
// message.
//
// Landlock requires Linux kernel 5.13 or newer; on older kernels the
// underlying syscall returns ENOSYS and [Manager.Load] fails with
// [ErrSandboxFailed]. Enabling Landlock also forces PR_SET_NO_NEW_PRIVS
// on the host process — the underlying [landlock.Apply] sets it as a
// kernel prerequisite, regardless of [SandboxOptions.NoNewPrivs]. Mirror
// of [config.PluginsLandlock]; convert via [SandboxOptionsFromConfig].
type LandlockOptions struct {
	// Enabled turns the Landlock pass on. Independent from the rlimits — the
	// surrounding [SandboxOptions.Enabled] must also be true for any sandbox
	// pass to run.
	Enabled bool

	// AllowPluginDir, when true, causes [Manager.ensureSandbox] to append
	// the configured plugin directory to [ReadPaths] before applying the
	// ruleset. Convenience flag for the common case where the operator
	// always wants the host to read and exec from the plugin directory.
	AllowPluginDir bool

	// AllowSystemLibs, when true, causes [Manager.ensureSandbox] to append
	// the standard system library directories (/lib, /lib64, /usr/lib,
	// /usr/lib64) to [ReadPaths] before applying the ruleset. Covers glibc
	// and musl on mainstream distros. Disable on NixOS, chroot jails, and
	// custom library-prefix deployments.
	AllowSystemLibs bool

	// ReadPaths grants read+execute on each listed path (and everything
	// beneath it for directories). Every entry must be an absolute path.
	// Merged with the auto-added entries from [AllowPluginDir] and
	// [AllowSystemLibs] at apply time; the runtime slice is cloned before
	// the merge so the caller's slice is never mutated.
	ReadPaths []string

	// ReadWritePaths grants read+execute+write+truncate on each listed path.
	// Every entry must be an absolute path.
	ReadWritePaths []string
}

// CapabilitiesOptions configures Linux capability dropping applied
// between the rlimits and Landlock passes when [SandboxOptions.Enabled]
// is true. Capabilities are dropped via the standalone
// [github.com/altessa-s/go-atlas/core/runtime/capabilities] package; this
// struct is a thin adapter so operator config maps cleanly to runtime
// options.
//
// When Enabled is true and Keep is empty, every capability is dropped
// — the "nuclear option" for plugin hosts that have no legitimate need
// for any privileged syscalls. A non-empty Keep preserves exactly those
// capabilities in the effective, permitted, and bounding sets (the
// inheritable and ambient sets are always cleared).
//
// Each Keep entry must match a canonical "CAP_*" name (case-insensitive;
// resolved via [capabilities.ParseName]). An unknown name fails
// [Manager.Load] with [ErrSandboxFailed] wrapping
// [capabilities.ErrInvalidOption]. Operator config is validated up-front
// in [config.PluginsCapabilities.Validate], so a properly-loaded config
// never reaches this stage with an unknown name.
//
// Mirror of [config.PluginsCapabilities]; convert via
// [SandboxOptionsFromConfig].
type CapabilitiesOptions struct {
	// Enabled turns the capability-dropping pass on. Independent from
	// the rlimits — the surrounding [SandboxOptions.Enabled] must also
	// be true for any sandbox pass to run.
	Enabled bool

	// Keep lists capability names to preserve. See the package-level
	// doc comment on [CapabilitiesOptions] for the expected format.
	Keep []string
}

// SandboxOptionsFromConfig converts a [config.PluginsSandbox] block into the
// runtime [SandboxOptions] consumed by the manager. The returned struct
// owns its own copies of the slice fields ([LandlockOptions.ReadPaths],
// [LandlockOptions.ReadWritePaths], [CapabilitiesOptions.Keep]) — mutating
// the input config after conversion is safe and does not affect the
// already-converted runtime options.
func SandboxOptionsFromConfig(c config.PluginsSandbox) SandboxOptions {
	return SandboxOptions{
		Enabled:          c.Enabled,
		NoNewPrivs:       c.NoNewPrivs,
		MemoryLimitBytes: c.MemoryLimitBytes,
		MaxOpenFiles:     c.MaxOpenFiles,
		MaxProcesses:     c.MaxProcesses,
		MaxFileSizeBytes: c.MaxFileSizeBytes,
		DisableCoreDumps: c.DisableCoreDumps,
		Capabilities: CapabilitiesOptions{
			Enabled: c.Capabilities.Enabled,
			Keep:    slices.Clone(c.Capabilities.Keep),
		},
		Landlock: LandlockOptions{
			Enabled:         c.Landlock.Enabled,
			AllowPluginDir:  c.Landlock.AllowPluginDir,
			AllowSystemLibs: c.Landlock.AllowSystemLibs,
			ReadPaths:       slices.Clone(c.Landlock.ReadPaths),
			ReadWritePaths:  slices.Clone(c.Landlock.ReadWritePaths),
		},
	}
}

// Validate reports whether the runtime sandbox options are well-formed
// enough for [Manager.Load] to apply them. Returns nil when the sandbox
// is disabled.
//
// Validate covers the operator-config invariants that ozzo-validation
// enforces in [config.PluginsSandbox.Validate], so callers that build
// SandboxOptions programmatically (rather than via
// [SandboxOptionsFromConfig]) get the same fail-fast behavior:
//
//   - rlimit values must be non-negative (zero is the documented
//     "unset" sentinel)
//   - Landlock paths must be non-empty absolute paths
//   - Capability names must parse via [capabilities.ParseName]
//
// Multiple violations are joined via [errors.Join] so the operator
// sees every problem at once instead of fixing them one by one.
func (o SandboxOptions) Validate() error {
	if !o.Enabled {
		return nil
	}
	var errs []error
	errs = coreslices.AppendIf(errs, o.MemoryLimitBytes < 0,
		fmt.Errorf("MemoryLimitBytes=%d is negative", o.MemoryLimitBytes))
	errs = coreslices.AppendIf(errs, o.MaxOpenFiles < 0,
		fmt.Errorf("MaxOpenFiles=%d is negative", o.MaxOpenFiles))
	errs = coreslices.AppendIf(errs, o.MaxProcesses < 0,
		fmt.Errorf("MaxProcesses=%d is negative", o.MaxProcesses))
	errs = coreslices.AppendIf(errs, o.MaxFileSizeBytes < 0,
		fmt.Errorf("MaxFileSizeBytes=%d is negative", o.MaxFileSizeBytes))
	errs = coreslices.AppendNonNil(errs, o.Capabilities.Validate())
	errs = coreslices.AppendNonNil(errs, o.Landlock.Validate())
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

// Validate checks the Landlock options for the same invariants enforced
// by [config.PluginsLandlock.Validate]: every path entry must be a
// non-empty absolute filesystem path. Validate is a no-op when Landlock
// is disabled.
func (o LandlockOptions) Validate() error {
	if !o.Enabled {
		return nil
	}
	var errs []error
	for i, p := range o.ReadPaths {
		errs = coreslices.AppendNonNil(errs, validateLandlockPath("Landlock.ReadPaths", i, p))
	}
	for i, p := range o.ReadWritePaths {
		errs = coreslices.AppendNonNil(errs, validateLandlockPath("Landlock.ReadWritePaths", i, p))
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

// Validate checks the capability options for the same invariants
// enforced by [config.PluginsCapabilities.Validate]: every entry in
// Keep must be a parseable canonical CAP_* name. Validate is a no-op
// when capability dropping is disabled.
func (o CapabilitiesOptions) Validate() error {
	if !o.Enabled {
		return nil
	}
	var errs []error
	for i, name := range o.Keep {
		if name == "" {
			errs = append(errs, fmt.Errorf("Capabilities.Keep[%d] is empty", i))
			continue
		}
		if _, err := capabilities.ParseName(name); err != nil {
			errs = append(errs, fmt.Errorf("Capabilities.Keep[%d]: %w", i, err))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

// validateLandlockPath enforces the "non-empty absolute path" rule on
// a single Landlock allowlist entry. The error message includes the
// field name and slice index so the operator can pinpoint the bad
// entry without re-reading the config.
func validateLandlockPath(field string, idx int, p string) error {
	if p == "" {
		return fmt.Errorf("%s[%d] is empty", field, idx)
	}
	if !filepath.IsAbs(p) {
		return fmt.Errorf("%s[%d] %q is not an absolute path", field, idx, p)
	}
	return nil
}
