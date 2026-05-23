// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_DefaultSignatureMode(t *testing.T) {
	t.Parallel()

	// With no options, the manager should default to SignatureRequire.
	mgr := NewManager()
	assert.Equal(t, SignatureRequire, mgr.opts.signature.mode)
}

func TestManager_LoadWithDefaultSignatureMode(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Create a fake .so file without a signature.
	pluginPath := filepath.Join(dir, "test.so")
	require.NoError(t, os.WriteFile(pluginPath, []byte("fake plugin"), 0o644))

	// Manager with default options (SignatureRequire) and a directory.
	mgr := NewManager(WithDir(dir))
	t.Cleanup(func() { _ = mgr.Close() })

	// Load should fail because no public key is configured
	// and SignatureRequire is the default mode.
	err := mgr.Load(t.Context())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSignatureConfig)
}

func TestManager_DisabledSignatureAllowsUnsigned(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Create a fake .so file without a signature.
	pluginPath := filepath.Join(dir, "test.so")
	require.NoError(t, os.WriteFile(pluginPath, []byte("fake plugin"), 0o644))

	// Manager with SignatureDisabled explicitly set.
	mgr := NewManager(
		WithDir(dir),
		WithSignatureDisabled(),
	)
	t.Cleanup(func() { _ = mgr.Close() })

	// Load will still fail (invalid .so format) but not due to signature.
	err := mgr.Load(t.Context())
	require.Error(t, err)
	// Should NOT be ErrSignatureConfig since we disabled signature checking.
	assert.NotErrorIs(t, err, ErrSignatureConfig)
}
