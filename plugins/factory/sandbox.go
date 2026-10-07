// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"slices"

	"github.com/altessa-s/go-atlas/plugins"

	pluginsconfig "github.com/altessa-s/go-atlas/config/plugins"
)

// SandboxOptionsFromConfig converts a [pluginsconfig.Sandbox] block into the
// runtime [plugins.SandboxOptions] consumed by the manager. The returned struct
// owns its own copies of the slice fields ([plugins.LandlockOptions.ReadPaths],
// [plugins.LandlockOptions.ReadWritePaths], [plugins.CapabilitiesOptions.Keep]) — mutating
// the input config after conversion is safe and does not affect the
// already-converted runtime options.
func SandboxOptionsFromConfig(c pluginsconfig.Sandbox) plugins.SandboxOptions {
	return plugins.SandboxOptions{
		Enabled:          c.Enabled,
		NoNewPrivs:       c.NoNewPrivs,
		MemoryLimitBytes: c.MemoryLimitBytes,
		MaxOpenFiles:     c.MaxOpenFiles,
		MaxProcesses:     c.MaxProcesses,
		MaxFileSizeBytes: c.MaxFileSizeBytes,
		DisableCoreDumps: c.DisableCoreDumps,
		Capabilities: plugins.CapabilitiesOptions{
			Enabled: c.Capabilities.Enabled,
			Keep:    slices.Clone(c.Capabilities.Keep),
		},
		Landlock: plugins.LandlockOptions{
			Enabled:         c.Landlock.Enabled,
			AllowPluginDir:  c.Landlock.AllowPluginDir,
			AllowSystemLibs: c.Landlock.AllowSystemLibs,
			ReadPaths:       slices.Clone(c.Landlock.ReadPaths),
			ReadWritePaths:  slices.Clone(c.Landlock.ReadWritePaths),
		},
	}
}
