// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package loader_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config/loader"
)

// TestLoad_RejectsSymlinkOutsideRoot is a regression test for symlink
// confinement: when the configured config directory contains a symlink
// pointing outside of it, the loader must refuse to follow rather than
// silently decoding the target. Without this check a writer with access to
// the config volume could drop e.g. `creds.yaml -> /etc/shadow` and the
// loader would happily ingest it.
func TestLoad_RejectsSymlinkOutsideRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows")
	}

	configDir := t.TempDir()
	outsideDir := t.TempDir()

	// Real, valid YAML living *outside* the config root.
	outsideTarget := filepath.Join(outsideDir, "leak.yaml")
	require.NoError(t, os.WriteFile(outsideTarget, []byte("appName: leaked\n"), 0o644))

	// A baseline file inside the root so the directory is otherwise valid.
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte("appName: ok\n"), 0o644))

	// The escape symlink. Its target has a valid extension and is real
	// YAML, so the only thing standing between the loader and a happy
	// decode is the confinement check.
	symlinkPath := filepath.Join(configDir, "escape.yaml")
	require.NoError(t, os.Symlink(outsideTarget, symlinkPath))

	cfg := &TestConfig{}
	l := loader.New(nil, loader.WithPath(configDir))

	_, err := l.Load(cfg)
	require.Error(t, err, "loader must reject a symlink whose target escapes the config root")
	require.ErrorIs(t, err, loader.ErrPathOutsideRoot)
}

// TestLoad_AllowsSymlinkInsideRoot confirms the confinement check is a
// containment check, not a blanket symlink ban: Kubernetes ConfigMap
// volumes mount config files via internal symlinks (`..data/foo.yaml` →
// `..2026_04_25_.../foo.yaml`), and we must keep working there.
func TestLoad_AllowsSymlinkInsideRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows")
	}

	configDir := t.TempDir()

	// Real file inside the root.
	target := filepath.Join(configDir, "real.yaml")
	require.NoError(t, os.WriteFile(target, []byte("appName: from-symlink\n"), 0o644))

	// Symlink alongside it, also inside the root.
	link := filepath.Join(configDir, "config.yaml")
	require.NoError(t, os.Symlink(target, link))

	cfg := &TestConfig{}
	l := loader.New(nil, loader.WithPath(configDir))

	_, err := l.Load(cfg)
	require.NoError(t, err, "in-root symlinks must continue to work (K8s ConfigMap pattern)")
	require.Equal(t, "from-symlink", cfg.AppName)
}
