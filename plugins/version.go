// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	"log/slog"
	"strings"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// majorOf extracts the major version segment from a semver string.
// Returns "" when version is empty or has no recognizable major
// (the loader treats either side returning "" as "did not declare",
// which skips the host-major check).
//
// Tolerates a leading "v" and trailing prerelease ("-…") or build
// metadata ("+…"). Intentionally simple: the loader only needs the
// major segment to enforce SemVer compatibility (same major implies
// ABI compatibility). Full semver parsing lives in
// [github.com/altessa-s/go-atlas/core/runtime/appinfo]; replicating
// it here keeps the plugins package free of an appinfo dependency.
func majorOf(version string) string {
	v := strings.TrimPrefix(version, "v")
	if i := strings.IndexAny(v, ".-+"); i >= 0 {
		v = v[:i]
	}
	return v
}

// checkHostMajor enforces or warns about a host major-version mismatch
// between [Descriptor.HostVersion] and the manager's configured host
// version. Returns nil when:
//
//   - the manager is in [HostVersionDisabled] mode, OR
//   - either side did not declare a version (opt-in semantics), OR
//   - the major components match.
//
// In [HostVersionWarn] mode a mismatch is logged and nil is returned
// so the loader still registers the plugin. In [HostVersionEnforce]
// mode a mismatch returns [ErrHostVersionMismatch] and the loader
// quarantines the plugin and excludes it from the registry.
func (m *Manager) checkHostMajor(desc *Descriptor) error {
	if m.opts.hostVersionMode == HostVersionDisabled {
		return nil
	}
	hostMajor := majorOf(m.opts.hostVersion)
	descMajor := majorOf(desc.HostVersion)
	if hostMajor == "" || descMajor == "" || hostMajor == descMajor {
		return nil
	}
	if m.opts.hostVersionMode == HostVersionWarn {
		m.logger.Warn("plugin host major version mismatch (warn mode)",
			slog.String("plugin", desc.Name),
			slog.String("plugin_host", desc.HostVersion),
			slog.String("host", m.opts.hostVersion),
			slog.String("plugin_major", descMajor),
			slog.String("host_major", hostMajor),
		)
		return nil
	}
	return coreerrs.Wrapf(ErrHostVersionMismatch,
		"built for host major %q, host is %q (plugin=%q host=%q)",
		descMajor, hostMajor, desc.HostVersion, m.opts.hostVersion)
}
