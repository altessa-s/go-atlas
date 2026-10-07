// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/altessa-s/go-atlas/plugins"
	"github.com/altessa-s/go-atlas/plugins/factory"

	pluginsconfig "github.com/altessa-s/go-atlas/config/plugins"
)

func TestSandboxOptionsFromConfig(t *testing.T) {
	t.Parallel()

	cfg := pluginsconfig.Sandbox{
		Enabled:          true,
		NoNewPrivs:       true,
		MemoryLimitBytes: 1 << 28,
		MaxOpenFiles:     1024,
		MaxProcesses:     32,
		MaxFileSizeBytes: 1 << 30,
		DisableCoreDumps: true,
		Landlock: pluginsconfig.Landlock{
			Enabled:        true,
			ReadPaths:      []string{"/lib64", "/usr/lib64"},
			ReadWritePaths: []string{"/var/lib/myservice"},
		},
	}
	got := factory.SandboxOptionsFromConfig(cfg)
	want := plugins.SandboxOptions{
		Enabled:          true,
		NoNewPrivs:       true,
		MemoryLimitBytes: 1 << 28,
		MaxOpenFiles:     1024,
		MaxProcesses:     32,
		MaxFileSizeBytes: 1 << 30,
		DisableCoreDumps: true,
		Landlock: plugins.LandlockOptions{
			Enabled:        true,
			ReadPaths:      []string{"/lib64", "/usr/lib64"},
			ReadWritePaths: []string{"/var/lib/myservice"},
		},
	}
	assert.Equal(t, want, got)
}

func TestSandboxOptionsFromConfig_MirrorsAllowFlags(t *testing.T) {
	t.Parallel()

	cfg := pluginsconfig.Sandbox{
		Enabled: true,
		Landlock: pluginsconfig.Landlock{
			Enabled:         true,
			AllowPluginDir:  true,
			AllowSystemLibs: true,
			ReadPaths:       []string{"/etc/myservice"},
		},
	}
	got := factory.SandboxOptionsFromConfig(cfg)
	assert.True(t, got.Landlock.AllowPluginDir)
	assert.True(t, got.Landlock.AllowSystemLibs)
}
