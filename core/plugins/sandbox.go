// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	"github.com/altessa-s/go-atlas/config"
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

	// MaxProcesses caps RLIMIT_NPROC. 0 = unset.
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
// runtime [SandboxOptions] consumed by the manager.
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
			Keep:    c.Capabilities.Keep,
		},
		Landlock: LandlockOptions{
			Enabled:         c.Landlock.Enabled,
			AllowPluginDir:  c.Landlock.AllowPluginDir,
			AllowSystemLibs: c.Landlock.AllowSystemLibs,
			ReadPaths:       c.Landlock.ReadPaths,
			ReadWritePaths:  c.Landlock.ReadWritePaths,
		},
	}
}
