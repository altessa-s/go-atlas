// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	observabilityconfig "github.com/altessa-s/go-atlas/config/observability"
)

func TestLoggerBuilder_ConfigurableOptions(t *testing.T) {
	t.Run("Sensitive tags, mask string and app group", func(t *testing.T) {
		var buf bytes.Buffer
		const format = "configurable-options-capture"
		RegisterHandler(format, func(_ io.Writer, _ *observabilityconfig.Logger, opts *slog.HandlerOptions) slog.Handler {
			return slog.NewJSONHandler(&buf, opts)
		})

		logger, err := New(&observabilityconfig.Logger{
			Level:         observabilityconfig.LoggerLevelInfo,
			OutputFormat:  format,
			SensitiveTags: []string{"password"},
			MaskString:    "[REDACTED]",
			AppGroupName:  "metadata",
		}).WithAppName("svc").Build()
		require.NoError(t, err)
		logger.Info("test", "password", "secret123")

		var result map[string]any
		require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
		require.NotEqual(t, "secret123", result["password"])
		require.Contains(t, result, "metadata", "expected group \"metadata\" to exist in log")
	})

	t.Run("Dynamic Log Levels", func(t *testing.T) {
		b := New(&observabilityconfig.Logger{Level: observabilityconfig.LoggerLevelInfo, OutputFormat: "dynamic-levels"})
		RegisterHandler("dynamic-levels", func(_ io.Writer, _ *observabilityconfig.Logger, opts *slog.HandlerOptions) slog.Handler {
			return slog.NewJSONHandler(io.Discard, opts)
		})
		require.Equal(t, slog.LevelInfo, b.GetLevel(), "before Build the configured level is reported")

		_, err := b.Build()
		require.NoError(t, err)
		b.SetLevel(slog.LevelWarn)
		require.Equal(t, slog.LevelWarn, b.GetLevel())
	})

	t.Run("Custom Handler Registration", func(t *testing.T) {
		const customFormat = "mock"
		formatCalled := false

		RegisterHandler(customFormat, func(w io.Writer, cfg *observabilityconfig.Logger, opts *slog.HandlerOptions) slog.Handler {
			formatCalled = true
			return slog.NewJSONHandler(w, opts)
		})

		cfg := &observabilityconfig.Logger{
			Level:        observabilityconfig.LoggerLevelInfo,
			OutputFormat: customFormat,
		}

		_, _ = New(cfg).Build()

		require.True(t, formatCalled, "expected custom handler factory to be called")
	})
}

func TestNewCatalog_ConfigAliases(t *testing.T) {
	t.Parallel()

	catalog, err := newCatalog()
	require.NoError(t, err)

	smart, err := catalog.Resolve("smart", nil)
	require.NoError(t, err)
	require.Equal(t, "pa*******23", smart("password123"))

	hash, err := catalog.Resolve("hash", map[string]any{"prefix": "user:"})
	require.NoError(t, err)
	require.Equal(t, "user:[HASH]", hash("secret"))

	for _, name := range []string{"cc", "cached_partial", "full", "email"} {
		_, err := catalog.Resolve(name, nil)
		require.NoError(t, err, name)
	}
}
