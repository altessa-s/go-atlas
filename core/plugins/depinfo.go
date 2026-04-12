// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	"fmt"
	"log/slog"
	"runtime/debug"
)

// ModDep is a single module dependency exported by a plugin for
// host-side version-skew detection.
type ModDep struct {
	// Path is the module path (e.g. "google.golang.org/grpc").
	Path string

	// Version is the module version (e.g. "v1.60.0").
	Version string
}

// DepInfo is a curated build-dependency snapshot that a plugin may
// optionally export as a package-level variable named "DepInfo".
// After [plugin.Open] and before Init, the manager compares it
// against the host's own [debug.ReadBuildInfo] to surface
// shared-dependency version mismatches as advisory warnings.
//
// Both the value form and the pointer form are accepted:
//
//	var DepInfo = plugins.NewDepInfoFromBuild()       // recommended
//	var DepInfo = &plugins.DepInfo{GoVersion: "…", …} // manual
type DepInfo struct {
	// GoVersion is the Go toolchain version the plugin was built with
	// (e.g. "go1.25.0"). Captured automatically by [NewDepInfoFromBuild].
	GoVersion string

	// Deps lists the plugin's module dependencies at build time.
	Deps []ModDep
}

// NewDepInfoFromBuild constructs a [*DepInfo] from the running binary's
// embedded build metadata. Plugin authors use it as:
//
//	var DepInfo = plugins.NewDepInfoFromBuild()
//
// Returns nil when build information is unavailable (e.g. when running
// under `go test` without module mode). This is safe — the manager
// treats a nil DepInfo the same as an absent symbol.
func NewDepInfoFromBuild() *DepInfo {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return nil
	}
	deps := make([]ModDep, 0, len(bi.Deps))
	for _, d := range bi.Deps {
		m := d
		if m.Replace != nil {
			m = m.Replace
		}
		deps = append(deps, ModDep{Path: m.Path, Version: m.Version})
	}
	return &DepInfo{
		GoVersion: bi.GoVersion,
		Deps:      deps,
	}
}

// checkDepInfo compares the plugin's dependency snapshot against the host's
// own build info and logs warnings for any shared modules whose versions
// diverge. Mismatches do not prevent loading — they are advisory — because
// Go's plugin loader enforces the hard ABI check at [plugin.Open] time.
//
// The function is a no-op when pluginDeps is nil (absent [DepInfo] symbol)
// or when the host's [hostBuildInfo] returns nil.
func checkDepInfo(logger *slog.Logger, pluginName string, pluginDeps *DepInfo, hostBI hostBuildInfo) {
	if pluginDeps == nil {
		return
	}

	bi := hostBI()
	if bi == nil {
		return
	}

	// Build host dep index: path → version.
	hostDeps := make(map[string]string, len(bi.Deps))
	for _, dep := range bi.Deps {
		d := dep
		if d.Replace != nil {
			d = d.Replace
		}
		hostDeps[d.Path] = d.Version
	}

	var mismatches []string
	for _, pd := range pluginDeps.Deps {
		if hostVer, ok := hostDeps[pd.Path]; ok && hostVer != pd.Version {
			mismatches = append(mismatches, fmt.Sprintf("%s: plugin=%s host=%s", pd.Path, pd.Version, hostVer))
		}
	}

	if len(mismatches) > 0 {
		logger.Warn("plugin has shared-dependency version mismatches with host; "+
			"this may cause runtime type assertion failures or plugin.Open errors",
			slog.String("plugin", pluginName),
			slog.Int("mismatches", len(mismatches)),
			slog.Any("details", mismatches),
		)
	}
}

// hostBuildInfo abstracts [debug.ReadBuildInfo] so tests can inject a
// canned build-info snapshot without depending on the test binary's
// actual embedded metadata.
type hostBuildInfo func() *debug.BuildInfo

// readHostBuildInfo is the production implementation of [hostBuildInfo].
func readHostBuildInfo() *debug.BuildInfo {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return nil
	}
	return bi
}
