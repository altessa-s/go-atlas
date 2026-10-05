// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/observability/slog/factory"

	slogx "github.com/altessa-s/go-atlas/observability/slog"
)

func TestModuleKey_MatchesSlogx(t *testing.T) {
	t.Parallel()

	require.Equal(t, slogx.ModuleKey, factory.ModuleKey)
}

// TestBuild_ModulePrefixStaysAtRoot checks that the default prefix key is the
// slogx.Module key and that the tag stays at the root after a WithGroup.
// Custom capture formats use the bracketed text formatter.
func TestBuild_ModulePrefixStaysAtRoot(t *testing.T) {
	t.Parallel()

	cfg := &config.Logger{Level: config.LoggerLevelInfo}
	b, _, buf := isolatedCapture(t, cfg)

	logger, err := b.Build()
	require.NoError(t, err)
	logger.With(slogx.Module("auth")).WithGroup("req").Info("m", "id", 1)

	record := decodeRecord(t, buf)
	require.Equal(t, "[auth]", record[slogx.ModuleKey])
	require.Equal(t, map[string]any{"id": float64(1)}, record["req"])
}

// TestBuild_ModulePrefixWithSubsystemLevels checks that the leveled handler
// still filters on the raw subsystem value while the prefix is rendered.
func TestBuild_ModulePrefixWithSubsystemLevels(t *testing.T) {
	t.Parallel()

	cfg := &config.Logger{
		Level:      config.LoggerLevelInfo,
		Subsystems: map[string]config.LoggerLevel{"auth": config.LoggerLevelDebug},
	}
	b, _, buf := isolatedCapture(t, cfg)

	logger, err := b.Build()
	require.NoError(t, err)
	logger.With(slogx.Module("db")).WithGroup("req").Debug("dropped")
	require.Zero(t, buf.Len())

	logger.With(slogx.Module("auth")).WithGroup("req").Debug("kept", "id", 1)
	record := decodeRecord(t, buf)
	require.Equal(t, "[auth]", record[slogx.ModuleKey])
	require.Equal(t, map[string]any{"id": float64(1)}, record["req"])
}

func TestBuild_WithPrefixKeyModuleKeepsOldBehavior(t *testing.T) {
	t.Parallel()

	cfg := &config.Logger{Level: config.LoggerLevelInfo}
	b, _, buf := isolatedCapture(t, cfg)

	logger, err := b.WithPrefixKey("module").Build()
	require.NoError(t, err)
	logger.Info("m", "module", "api", slogx.Module("auth"))

	record := decodeRecord(t, buf)
	require.Equal(t, "[api]", record["module"])
	require.Equal(t, "auth", record[slogx.ModuleKey])
}
