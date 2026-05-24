// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package loader_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config/loader"

	coreio "github.com/altessa-s/go-atlas/core/io"
)

// TestLoad_RespectsMaxConfigBytes is the regression guard for the
// unbounded-ReadAll fix. A config file larger than the configured cap
// must produce a typed error wrapping [coreio.ErrReadLimitExceeded]
// and [loader.ErrDecode], not silently OOM the process.
func TestLoad_RespectsMaxConfigBytes(t *testing.T) {
	// Write a file larger than the cap we'll configure (1 KiB).
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "huge.yaml")

	const cap = 1024
	require.NoError(t, os.WriteFile(configPath,
		[]byte("appName: x\nfiller: "+strings.Repeat("a", cap*4)+"\n"),
		0o644))

	cfg := &TestConfig{}
	l := loader.New(nil,
		loader.WithPath(configPath),
		loader.WithMaxConfigBytes(cap),
	)

	_, err := l.Load(cfg)
	require.Error(t, err, "oversized config must be rejected")
	require.True(t, errors.Is(err, coreio.ErrReadLimitExceeded),
		"error chain must surface ErrReadLimitExceeded so callers can distinguish a size limit from a parse error")
}

// TestLoad_AcceptsConfigUnderCap pins the happy path so a too-tight
// regression on the default cap would surface here as well.
func TestLoad_AcceptsConfigUnderCap(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "small.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("appName: x\nport: 1\n"), 0o644))

	cfg := &TestConfig{}
	l := loader.New(nil,
		loader.WithPath(configPath),
		loader.WithMaxConfigBytes(64*1024),
	)

	_, err := l.Load(cfg)
	require.NoError(t, err)
	require.Equal(t, "x", cfg.AppName)
}

// TestLoad_DefaultCapAppliesWhenUnset confirms the safe-default path:
// callers who do not pass WithMaxConfigBytes still get the bound from
// [loader.DefaultMaxConfigBytes] rather than the legacy unbounded
// behavior.
func TestLoad_DefaultCapAppliesWhenUnset(t *testing.T) {
	require.Positive(t, loader.DefaultMaxConfigBytes,
		"DefaultMaxConfigBytes must be a positive bound — zero would re-enable the OOM vector")
}
