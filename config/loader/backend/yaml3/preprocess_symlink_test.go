// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package yaml3_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config/loader/backend/yaml3"
)

// TestBackend_Preprocess_SymlinkTraversalBlocked is the primary
// regression guard for the path-traversal-via-symlink fix. A symlink
// inside rootDir pointing to a file OUTSIDE rootDir must be rejected
// with a "security error" — before the fix the cleaned path-based
// Rel-check passed and os.ReadFile followed the symlink to the
// outside target.
func TestBackend_Preprocess_SymlinkTraversalBlocked(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows; the loader is targeted at POSIX deployments")
	}

	// Real target lives OUTSIDE rootDir.
	outsideDir := t.TempDir()
	secret := filepath.Join(outsideDir, "secret.yaml")
	require.NoError(t, os.WriteFile(secret, []byte("leaked: true\n"), 0o644))

	// Config root contains a symlink that LOOKS like a legitimate
	// include candidate (the symlink name ends in .yaml) but actually
	// points to the secret outside the root.
	rootDir := t.TempDir()
	symlink := filepath.Join(rootDir, "legit.yaml")
	require.NoError(t, os.Symlink(secret, symlink))

	mainContent := "!include legit.yaml\n"
	backend := &yaml3.Backend{}
	result, err := backend.Preprocess(mainContent, rootDir, rootDir)

	require.Error(t, err, "symlink to outside rootDir MUST be rejected — this is the audit finding")
	require.Contains(t, err.Error(), "security error",
		"the error must carry the security-error prefix so operators can grep for it")
	require.Empty(t, result, "no content must be emitted when the security check fires")
}

// TestBackend_Preprocess_SymlinkToNonYAMLBlocked covers the second
// security control added by the fix: even if a symlink stays inside
// rootDir, its resolved target must still be a YAML file. A symlink
// `legit.yaml -> something-without-yaml-extension` inside rootDir was
// previously accepted because the original extension check only saw
// the symlink name; the re-check after EvalSymlinks catches it now.
func TestBackend_Preprocess_SymlinkToNonYAMLBlocked(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows")
	}

	rootDir := t.TempDir()
	plain := filepath.Join(rootDir, "plain.txt")
	require.NoError(t, os.WriteFile(plain, []byte("not yaml\n"), 0o644))

	symlink := filepath.Join(rootDir, "fake.yaml")
	require.NoError(t, os.Symlink(plain, symlink))

	mainContent := "!include fake.yaml\n"
	backend := &yaml3.Backend{}
	_, err := backend.Preprocess(mainContent, rootDir, rootDir)

	require.Error(t, err)
	require.Contains(t, err.Error(), "non-YAML target",
		"the resolved-target extension check must reject symlinks to non-YAML files")
}

// TestBackend_Preprocess_SymlinkInsideRootStillWorks pins the happy
// path: a symlink to another .yaml file inside rootDir must continue
// to work. Tightening the security checks should not regress
// legitimate symlink-based config organization (e.g. environment-
// specific overlays).
func TestBackend_Preprocess_SymlinkInsideRootStillWorks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows")
	}

	rootDir := t.TempDir()

	target := filepath.Join(rootDir, "target.yaml")
	require.NoError(t, os.WriteFile(target, []byte("included: real\n"), 0o644))

	symlink := filepath.Join(rootDir, "alias.yaml")
	require.NoError(t, os.Symlink(target, symlink))

	mainContent := "!include alias.yaml\n"
	backend := &yaml3.Backend{}
	result, err := backend.Preprocess(mainContent, rootDir, rootDir)

	require.NoError(t, err, "symlinks WITHIN rootDir to .yaml targets must continue to work")
	require.Contains(t, result, "included: real",
		"the included content must be inlined as before")
}
