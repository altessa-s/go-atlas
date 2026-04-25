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

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/observability/slog/handler/masking"

	slogx "github.com/altessa-s/go-atlas/observability/slog"
)

func TestLoggerBuilder_ConfigurableOptions(t *testing.T) {
	t.Run("Default Mask String", func(t *testing.T) {
		replaceAttr := slogx.MaskingReplaceAttr([]string{"password"}, "****")
		attr := replaceAttr(nil, slog.String("password", "secret123"))

		require.Equal(t, "****", attr.Value.String())
	})

	t.Run("Config Overrides Masking and Groups", func(t *testing.T) {
		b := New(nil)

		cfg := &config.Logger{
			Level:         config.LoggerLevelInfo,
			OutputFormat:  config.LogFormatJSON,
			SensitiveTags: []string{"password"},
			MaskString:    "[REDACTED]",
			AppGroupName:  "metadata",
		}

		maskString := cfg.MaskString
		appGroupName := cfg.AppGroupName

		replaceAttr := slogx.MaskingReplaceAttr(cfg.SensitiveTags, maskString)
		attr := replaceAttr(nil, slog.String("password", "secret123"))
		require.Equal(t, "[REDACTED]", attr.Value.String())

		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, nil)
		logger := slog.New(handler)
		logger = b.addAppMetadata(logger, appGroupName)
		logger.Info("test")

		var result map[string]any
		require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
		require.Contains(t, result, "metadata", "expected group \"metadata\" to exist in log")
	})

	t.Run("Custom Prefix Colors", func(t *testing.T) {
		colors := map[string][]int{"custom": {1, 2, 3}}
		b := New(nil).WithPrefixColors(colors)

		require.Len(t, b.prefixColors, 1)
		require.Equal(t, 1, b.prefixColors["custom"][0])
	})

	t.Run("Dynamic Log Levels", func(t *testing.T) {
		levelVar := &slog.LevelVar{}
		b := New(nil).WithLevelVar(levelVar)

		b.SetLevel(slog.LevelWarn)
		require.Equal(t, slog.LevelWarn, levelVar.Level())
		require.Equal(t, slog.LevelWarn, b.GetLevel())
	})

	t.Run("Custom Handler Registration", func(t *testing.T) {
		const customFormat = "mock"
		formatCalled := false

		RegisterHandler(customFormat, func(w io.Writer, cfg *config.Logger, opts *slog.HandlerOptions) slog.Handler {
			formatCalled = true
			return slog.NewJSONHandler(w, opts)
		})

		cfg := &config.Logger{
			Level:        config.LoggerLevelInfo,
			OutputFormat: customFormat,
		}

		_, _ = New(cfg).Build()

		require.True(t, formatCalled, "expected custom handler factory to be called")
	})
}

func TestMaskingHandler(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, nil)

	handler := masking.NewHandler(inner, masking.WithField("ssn", masking.FullMask()))
	require.NotNil(t, handler)
}
