// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package pluginsconfig_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pluginsconfig "github.com/altessa-s/go-atlas/config/plugins"
)

// These tests cover the Validate methods on the operator-config types
// in this package. They previously lived in the plugin host
// (plugins/sandbox_test.go, now github.com/altessa-s/go-proteus) but the coverage was being recorded against the wrong package — the
// methods under test are defined here, so the tests belong here too.

func TestPluginsSandbox_Validate_RejectsNegative(t *testing.T) {
	cases := []struct {
		name string
		cfg  pluginsconfig.Sandbox
	}{
		{
			name: "negative MemoryLimitBytes",
			cfg:  pluginsconfig.Sandbox{Enabled: true, MemoryLimitBytes: -1},
		},
		{
			name: "negative MaxOpenFiles",
			cfg:  pluginsconfig.Sandbox{Enabled: true, MaxOpenFiles: -1},
		},
		{
			name: "negative MaxProcesses",
			cfg:  pluginsconfig.Sandbox{Enabled: true, MaxProcesses: -1},
		},
		{
			name: "negative MaxFileSizeBytes",
			cfg:  pluginsconfig.Sandbox{Enabled: true, MaxFileSizeBytes: -1},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Error(t, tc.cfg.Validate())
		})
	}
}

func TestPluginsSandbox_Validate_DisabledSkipsChecks(t *testing.T) {
	// Negative values are accepted when the sandbox is disabled — the
	// values are dead config. Same convention as the rest of go-atlas
	// (see ValidateStructIfEnabled).
	cfg := pluginsconfig.Sandbox{Enabled: false, MemoryLimitBytes: -1}
	assert.NoError(t, cfg.Validate())
}

func TestPluginsSandbox_Validate_AcceptsZeroAndPositive(t *testing.T) {
	cfg := pluginsconfig.Sandbox{
		Enabled:          true,
		NoNewPrivs:       true,
		MemoryLimitBytes: 0, // unset
		MaxOpenFiles:     1024,
	}
	assert.NoError(t, cfg.Validate())
}

func TestPluginsLandlock_Validate_DisabledSkipsChecks(t *testing.T) {
	// Even with bogus paths, a disabled Landlock block must validate cleanly.
	cfg := pluginsconfig.Landlock{
		Enabled:   false,
		ReadPaths: []string{"relative/path", ""},
	}
	assert.NoError(t, cfg.Validate())
}

func TestPluginsLandlock_Validate_RejectsRelativePaths(t *testing.T) {
	cases := []struct {
		name string
		cfg  pluginsconfig.Landlock
	}{
		{
			name: "relative read path",
			cfg: pluginsconfig.Landlock{
				Enabled:   true,
				ReadPaths: []string{"./etc"},
			},
		},
		{
			name: "relative read-write path",
			cfg: pluginsconfig.Landlock{
				Enabled:        true,
				ReadWritePaths: []string{"var/lib"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Error(t, tc.cfg.Validate())
		})
	}
}

func TestPluginsLandlock_Validate_RejectsEmptyPath(t *testing.T) {
	cfg := pluginsconfig.Landlock{
		Enabled:   true,
		ReadPaths: []string{""},
	}
	assert.Error(t, cfg.Validate())
}

func TestPluginsLandlock_Validate_AcceptsAbsolutePaths(t *testing.T) {
	cfg := pluginsconfig.Landlock{
		Enabled:        true,
		ReadPaths:      []string{"/lib64", "/usr/lib64", "/etc/myservice"},
		ReadWritePaths: []string{"/var/lib/myservice"},
	}
	assert.NoError(t, cfg.Validate())
}

func TestPluginsLandlock_Validate_EmptyAllowlistsAreAllowed(t *testing.T) {
	// An enabled Landlock with empty path lists is technically valid — it
	// locks the host out of the entire filesystem. The validator does not
	// second-guess that intent; it only checks that supplied paths are
	// well-formed. Operational sanity is the operator's responsibility.
	cfg := pluginsconfig.Landlock{Enabled: true}
	assert.NoError(t, cfg.Validate())
}

func TestPluginsConfig_Validate_RevokedHashes(t *testing.T) {
	t.Parallel()
	const digest = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
	for _, tc := range []struct {
		name    string
		hashes  []string
		wantErr bool
	}{
		{name: "none", hashes: nil},
		{name: "lower hex", hashes: []string{digest}},
		{name: "upper hex", hashes: []string{strings.ToUpper(digest)}},
		{name: "empty entry", hashes: []string{""}, wantErr: true},
		{name: "too short", hashes: []string{digest[:63]}, wantErr: true},
		{name: "not hex", hashes: []string{"z" + digest[1:]}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := pluginsconfig.Default()
			cfg.Enabled = true
			cfg.RevokedHashes = tc.hashes
			err := cfg.Validate()
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}
