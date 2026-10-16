// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build linux && cgo && !race

package plugins

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/encoding/hash"
	"github.com/altessa-s/go-atlas/core/runtime/landlock"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// Opt in because this builds a real shared object and requires Landlock in
// the kernel/container policy. Irreversible restrictions run only in children.
func TestSnapshotLandlockIntegration(t *testing.T) {
	t.Parallel()
	if os.Getenv("ATLAS_PLUGIN_INTEGRATION") != "1" {
		t.Skip("set ATLAS_PLUGIN_INTEGRATION=1 on Linux with cgo and Landlock")
	}
	if root := os.Getenv("ATLAS_PLUGIN_CHILD_ROOT"); root != "" {
		runSnapshotLandlockChild(t, root)
		return
	}
	root := t.TempDir()
	source := filepath.Join(root, "fixture.go")
	require.NoError(t, os.WriteFile(source, []byte("package main\nvar count int\nfunc init(){count++}\nfunc Count() int{return count}\n"), 0o600))
	cmd := exec.CommandContext(t.Context(), "go", "build", "-buildmode=plugin", "-o", filepath.Join(root, "fixture.so"), source)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
	for _, mode := range []string{"disabled", "parent-allowed", "parent-denied"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			base := t.TempDir()
			cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestSnapshotLandlockIntegration$", "-test.v")
			cmd.Env = append(os.Environ(), "ATLAS_PLUGIN_CHILD_ROOT="+root, "ATLAS_PLUGIN_CHILD_BASE="+base, "ATLAS_PLUGIN_CHILD_MODE="+mode)
			out, err := cmd.CombinedOutput()
			require.NoError(t, err, "%s", out)
			t.Logf("child result:\n%s", out)
		})
	}
}

func runSnapshotLandlockChild(t *testing.T, root string) {
	t.Helper()
	// Landlock is per-thread. Keep verification, dlopen and cleanup on the
	// exact OS thread that installs the policy; the child exits afterward.
	runtime.LockOSThread()
	base := os.Getenv("ATLAS_PLUGIN_CHILD_BASE")
	mode := os.Getenv("ATLAS_PLUGIN_CHILD_MODE")
	allowParent := mode != "parent-denied"
	data, err := os.ReadFile(filepath.Join(root, "fixture.so"))
	require.NoError(t, err)
	pub, priv := testhelpers.GenerateEd25519Key(t)
	// Each child owns its source/signature to avoid sibling writes.
	pluginDir := filepath.Join(base, "source")
	require.NoError(t, os.Mkdir(pluginDir, 0o700))
	writeSignedPlugin(t, pluginDir, "fixture.so", priv, data)
	policy := LandlockOptions{Enabled: mode != "disabled", ReadPaths: []string{"/"}}
	if allowParent {
		policy.ReadWritePaths = []string{base}
	}
	mgr := NewManager(WithDir(pluginDir), WithSnapshotDir(base), WithSignature(SignatureOptions{Mode: SignatureRequire, PublicKey: pub}),
		WithSandbox(SandboxOptions{Enabled: mode != "disabled", Landlock: policy}))
	// This fixture intentionally has no toolkit descriptor. It must run init
	// successfully before descriptor resolution returns this sentinel.
	err = mgr.Load(t.Context())
	if errors.Is(err, landlock.ErrUnsupported) {
		t.Skipf("kernel does not support Landlock: %v", err)
	}
	require.ErrorIs(t, err, ErrNoDescriptor)
	dir := mgr.currentSnapshotDir()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries, "snapshot must be unlinked under Landlock")
	raw, err := mgr.openVerified("fixture.so", data, hash.SHA256HexBytes(data))
	require.NoError(t, err)
	sym, err := raw.Lookup("Count")
	require.NoError(t, err)
	count, ok := sym.(func() int)
	require.True(t, ok)
	require.Equal(t, 1, count())
	if allowParent {
		require.NoError(t, mgr.Close())
		require.NoDirExists(t, dir)
		other := NewManager(WithDir(pluginDir), WithSnapshotDir(base), WithSignature(SignatureOptions{Mode: SignatureRequire, PublicKey: pub}))
		require.ErrorIs(t, other.Load(t.Context()), ErrNoDescriptor)
		require.Equal(t, 1, count(), "another manager must reuse the runtime handle")
		require.NoError(t, other.Close())
	} else {
		require.ErrorIs(t, mgr.Close(), os.ErrPermission)
		require.Equal(t, dir, mgr.currentSnapshotDir())
		require.DirExists(t, dir, "parent test cleans up after restricted child exits")
	}
}
