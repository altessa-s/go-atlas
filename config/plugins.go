// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"path/filepath"
	"time"

	"github.com/altessa-s/go-atlas/core/runtime/capabilities"

	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for Plugins configuration.
const (
	defaultPluginsDir           = "./plugins"
	defaultPluginsInitTimeout   = 5 * time.Second
	defaultPluginsWatchDebounce = 200 * time.Millisecond
)

// Plugins defines configuration for the dynamic plugin manager.
// It controls which .so plugins are loaded from the plugin directory.
type Plugins struct {
	// Enabled controls whether the plugin manager is active.
	// Defaults to false.
	Enabled bool `yaml:"enabled" default:"false"`

	// Dir is the directory to scan for .so plugin files.
	// Defaults to "./plugins".
	Dir string `yaml:"dir" default:"./plugins"`

	// Load is an explicit allowlist of plugin filenames to load.
	// When non-empty, only these plugins are loaded and Disabled is ignored.
	Load []string `yaml:"load"`

	// Disabled is an exclusion list of plugin filenames to skip.
	// When Load is empty, all plugins from Dir are loaded except those listed here.
	Disabled []string `yaml:"disabled"`

	// InitTimeout is the maximum duration allowed for a plugin's Init call.
	// Defaults to 5s.
	InitTimeout time.Duration `yaml:"initTimeout" default:"5s"`

	// Watch enables a filesystem watcher that picks up new .so files added
	// to Dir at runtime and loads them automatically. Modifications and
	// removals are not acted upon because Go's plugin package cannot unload
	// or replace code that is already mapped into the process.
	// Defaults to false.
	Watch bool `yaml:"watch" default:"false"`

	// WatchDebounce coalesces rapid filesystem events into a single reload.
	// Only consulted when Watch is true.
	// Defaults to 200ms.
	WatchDebounce time.Duration `yaml:"watchDebounce" default:"200ms"`

	// Sandbox configures Linux process-hardening primitives applied lazily on
	// the first [plugins.Manager.Load]. See [PluginsSandbox] for the threat
	// model and the very real limitations.
	Sandbox PluginsSandbox `yaml:"sandbox"`
}

// PluginsSandbox configures Linux process-hardening primitives that the plugin
// manager applies before opening any .so file.
//
// Important caveats:
//
//   - Linux only. On other platforms an enabled sandbox makes Manager.Load
//     fail with [plugins.ErrSandboxUnsupported].
//   - Applied lazily on the first Manager.Load and irreversible. Host code
//     that runs before Load is not restricted.
//   - The primitives are process-wide. Every limit applies to the entire host
//     process, not just plugin code. Setting MaxOpenFiles too low will starve
//     a database connection pool the host owns.
//   - This is defense-in-depth, not a security boundary. A malicious plugin
//     still has full access to host memory and can corrupt or exfiltrate
//     anything in the address space.
type PluginsSandbox struct {
	// Enabled turns the sandbox on. When false, every other field is ignored.
	// Defaults to false.
	Enabled bool `yaml:"enabled" default:"false"`

	// NoNewPrivs sets PR_SET_NO_NEW_PRIVS so the process and any binary it
	// later execs cannot gain privileges via SUID/SGID. Recommended; has no
	// downside on a non-setuid host. Note: enabling [PluginsLandlock] also
	// forces NO_NEW_PRIVS unconditionally (it is a kernel prerequisite for
	// landlock_restrict_self), so this field is only meaningful when
	// Landlock is disabled.
	// Defaults to true.
	NoNewPrivs bool `yaml:"noNewPrivs" default:"true"`

	// MemoryLimitBytes caps the process's virtual address space (RLIMIT_AS).
	// 0 = unset. Counts every allocation in the host process, including the
	// Go runtime, in-process database drivers, and connection pools.
	MemoryLimitBytes int64 `yaml:"memoryLimitBytes"`

	// MaxOpenFiles caps the number of open file descriptors (RLIMIT_NOFILE).
	// 0 = unset. Counts every host fd: sockets, files, pipes, epoll, etc.
	MaxOpenFiles int64 `yaml:"maxOpenFiles"`

	// MaxProcesses caps the number of processes the user owning this process
	// can create (RLIMIT_NPROC). 0 = unset.
	MaxProcesses int64 `yaml:"maxProcesses"`

	// MaxFileSizeBytes caps the maximum size of any file the process can write
	// (RLIMIT_FSIZE). 0 = unset.
	MaxFileSizeBytes int64 `yaml:"maxFileSizeBytes"`

	// DisableCoreDumps sets RLIMIT_CORE = 0 so the process cannot leak its
	// memory contents (including secrets) via a core dump after a crash.
	// Defaults to false.
	DisableCoreDumps bool `yaml:"disableCoreDumps" default:"false"`

	// Capabilities configures Linux capability dropping applied between
	// the rlimits and Landlock passes. See [PluginsCapabilities] for the
	// operator contract and caveats. Disabled by default — the plugin
	// sandbox stays backwards-compatible with existing deployments.
	Capabilities PluginsCapabilities `yaml:"capabilities"`

	// Landlock configures kernel-enforced filesystem allowlisting via the
	// Linux Landlock LSM (kernel >= 5.13). It is a strict allowlist: nothing
	// is auto-added. See [PluginsLandlock] for the operator contract and
	// caveats.
	Landlock PluginsLandlock `yaml:"landlock"`
}

// PluginsLandlock is a strict filesystem allowlist applied via Linux Landlock
// (kernel >= 5.13). Once enabled, the host process can only read, execute or
// write paths explicitly listed below. Anything not listed returns EACCES.
//
// IMPORTANT — read every line before enabling:
//
//   - Linux 5.13+ only. On older kernels Manager.Load fails with
//     [plugins.ErrSandboxFailed] wrapping the kernel error.
//   - Strict allowlist with NO auto-added paths. Operators MUST list every
//     filesystem path the host needs to function, including the plugin
//     directory, the dynamic loader (e.g. /lib64/ld-linux-x86-64.so.2 on
//     glibc x86_64), libc, every shared library the plugin imports, the
//     host's working directory, log files, config files, and any data
//     directories.
//   - Process-wide and irreversible. Restricts the host as much as plugin
//     code; there is no in-process undo.
//   - Misconfiguration usually surfaces as plugin.Open failing with
//     "permission denied" on the loader or libc, NOT as a clear "you forgot
//     /lib64". Test on a canary replica before fleet rollout.
//   - Requires Sandbox.Enabled = true; Landlock is a sub-feature.
//   - Implicitly forces PR_SET_NO_NEW_PRIVS on the host process. The
//     underlying landlock.Apply call sets it as a kernel prerequisite for
//     landlock_restrict_self, regardless of [PluginsSandbox.NoNewPrivs].
type PluginsLandlock struct {
	// Enabled turns Landlock on. When false, every other field is ignored.
	// Defaults to false.
	Enabled bool `yaml:"enabled" default:"false"`

	// AllowPluginDir, when true, automatically adds [Plugins.Dir] to the
	// Landlock ReadPaths allowlist. Nearly every deployment needs this —
	// without it, the manager cannot `dlopen` any .so file. Left off by
	// default to preserve the strict-allowlist contract; operators who want
	// the convenience opt in explicitly.
	// Defaults to false.
	AllowPluginDir bool `yaml:"allowPluginDir" default:"false"`

	// AllowSystemLibs, when true, automatically adds the common system
	// library directories (/lib, /lib64, /usr/lib, /usr/lib64) to the
	// Landlock ReadPaths allowlist. Covers glibc and musl on RHEL, Debian,
	// Ubuntu, Fedora, Alpine, and most other mainstream distros. NOT
	// suitable for NixOS, chroot jails, custom library prefixes, or any
	// deployment where the dynamic loader lives outside those paths — use
	// explicit ReadPaths on those systems.
	// Defaults to false.
	AllowSystemLibs bool `yaml:"allowSystemLibs" default:"false"`

	// ReadPaths grants read+execute on each listed path (and everything
	// beneath it for directories). Use this for the plugin directory, the
	// dynamic loader, libc, shared libraries, and any read-only data files.
	// Every entry must be an absolute path. Merged with the auto-added
	// entries from AllowPluginDir and AllowSystemLibs at apply time.
	ReadPaths []string `yaml:"readPaths"`

	// ReadWritePaths grants read+execute+write+truncate on each listed path.
	// Use this for log directories, runtime data directories, and anything
	// the host or its plugins need to mutate. Every entry must be an
	// absolute path.
	ReadWritePaths []string `yaml:"readWritePaths"`
}

// PluginsCapabilities configures Linux capability dropping applied via the
// standalone [github.com/altessa-s/go-atlas/core/runtime/capabilities]
// package. It is a defense-in-depth primitive: if the host process is
// launched with effective capabilities (file capabilities, systemd
// AmbientCapabilities=, docker --cap-add, or plain root), a plugin
// loaded via dlopen would otherwise inherit them. Dropping caps closes
// that vector before any untrusted code runs.
//
// IMPORTANT — read every line before enabling:
//
//   - Linux only. On non-Linux platforms the sandbox fails Manager.Load
//     with [plugins.ErrSandboxUnsupported] when capabilities are enabled.
//   - Process-wide in practice. [capabilities.DropAll] targets a single
//     thread, but the plugin sandbox applies it before any plugin
//     goroutine is spawned, so every thread the plugin later creates
//     inherits the reduced set.
//   - Irreversible. Once applied, capability ceilings cannot be raised
//     for the lifetime of the process.
//   - Requires Sandbox.Enabled = true; capability dropping is a
//     sub-feature of the umbrella sandbox like Landlock.
type PluginsCapabilities struct {
	// Enabled turns Linux capability dropping on. When false, every
	// other field is ignored.
	// Defaults to false.
	Enabled bool `yaml:"enabled" default:"false"`

	// Keep lists capability names to preserve in the effective,
	// permitted, and bounding sets. Each entry must match a canonical
	// CAP_* name (case-insensitive; parsed via
	// [capabilities.ParseName] at config-load time). An unknown name
	// fails Validate up-front.
	//
	// Leaving Keep empty with Enabled=true drops every capability,
	// which is what most plugin hosts want. A non-empty list is the
	// "bind a privileged port then drop everything else" pattern —
	// typical value: ["CAP_NET_BIND_SERVICE"].
	//
	// The inheritable and ambient sets are always cleared; there is
	// no operator knob for them because plugin hosts never need to
	// propagate capabilities across execve.
	Keep []string `yaml:"keep"`
}

// DefaultPlugins returns a Plugins configuration with default values.
func DefaultPlugins() Plugins {
	return Plugins{
		Dir:           defaultPluginsDir,
		InitTimeout:   defaultPluginsInitTimeout,
		WatchDebounce: defaultPluginsWatchDebounce,
	}
}

// IsEnabled reports whether the plugin manager should be activated.
// It returns false for a nil receiver so callers can chain
// cfg.Plugins.IsEnabled() without a separate nil guard.
func (c *Plugins) IsEnabled() bool {
	return c != nil && c.Enabled
}

// Validate checks the Plugins configuration for consistency.
func (c *Plugins) Validate() error {
	return ValidateStructIfEnabled(c.Enabled, c,
		validation.Field(&c.Dir, validation.Required),
		validation.Field(&c.InitTimeout, ozzo_rules.Duration(), validation.Min(time.Millisecond)),
		validation.Field(&c.WatchDebounce,
			validation.When(c.Watch, ozzo_rules.Duration(), validation.Min(time.Millisecond))),
		validation.Field(&c.Sandbox),
	)
}

// Validate checks the sandbox configuration for consistency. Negative limits
// are rejected; zero means "unset" and is allowed. Pointer receiver matches
// the convention used by every other config.*.Validate method in the project.
func (c *PluginsSandbox) Validate() error {
	if !c.Enabled {
		return nil
	}
	return validation.ValidateStruct(c,
		validation.Field(&c.MemoryLimitBytes, validation.Min(int64(0))),
		validation.Field(&c.MaxOpenFiles, validation.Min(int64(0))),
		validation.Field(&c.MaxProcesses, validation.Min(int64(0))),
		validation.Field(&c.MaxFileSizeBytes, validation.Min(int64(0))),
		validation.Field(&c.Capabilities),
		validation.Field(&c.Landlock),
	)
}

// Validate checks the capability-dropping configuration for consistency.
// Disabled capability dropping skips every check. When enabled, every
// entry in Keep must resolve to a known Linux capability via
// [capabilities.ParseName]; unknown names fail fast at config load
// rather than at Manager.Load time. The cross-field requirement that
// [PluginsSandbox.Enabled] must be true is enforced upstream.
func (c *PluginsCapabilities) Validate() error {
	if !c.Enabled {
		return nil
	}
	return validation.ValidateStruct(c,
		validation.Field(&c.Keep, validation.Each(
			validation.Required,
			validation.By(capabilityNameRule),
		)),
	)
}

// capabilityNameRule is an ozzo-validation [validation.RuleFunc] that
// rejects unknown capability names. Empty values are passed through;
// combine with [validation.Required] to reject those separately.
func capabilityNameRule(value any) error {
	s, ok := value.(string)
	if !ok || s == "" {
		return nil
	}
	if _, err := capabilities.ParseName(s); err != nil {
		return err
	}
	return nil
}

// Validate checks the Landlock configuration for consistency. Disabled
// Landlock skips every check. When enabled, every path entry must be a
// non-empty absolute path. The cross-field requirement that
// [PluginsSandbox.Enabled] must be true is enforced upstream:
// [PluginsSandbox.Validate] short-circuits when the parent sandbox is off,
// so this method's checks only run when the umbrella sandbox is on.
func (c *PluginsLandlock) Validate() error {
	if !c.Enabled {
		return nil
	}
	return validation.ValidateStruct(c,
		validation.Field(&c.ReadPaths, validation.Each(
			validation.Required,
			validation.By(absolutePathRule),
		)),
		validation.Field(&c.ReadWritePaths, validation.Each(
			validation.Required,
			validation.By(absolutePathRule),
		)),
	)
}

// absolutePathRule is an ozzo-validation [validation.RuleFunc] that rejects
// non-absolute filesystem paths. Empty values are passed through; combine
// with [validation.Required] to reject those separately.
func absolutePathRule(value any) error {
	s, ok := value.(string)
	if !ok || s == "" {
		return nil
	}
	if !filepath.IsAbs(s) {
		return validation.NewError("validation_absolute_path",
			"must be an absolute path")
	}
	return nil
}
