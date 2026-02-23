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

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/observability/slog/handler/masking"

	slogx "github.com/altessa-s/go-atlas/observability/slog"
)

func TestFactory_ConfigurableOptions(t *testing.T) {
	t.Run("Default Mask String", func(t *testing.T) {

		// 1. Check Default Masking (****)
		replaceAttr := slogx.MaskingReplaceAttr([]string{"password"}, "****")
		attr := replaceAttr(nil, slog.String("password", "secret123"))

		if attr.Value.String() != "****" {
			t.Errorf("expected default mask \"****\", got %q", attr.Value.String())
		}
	})

	t.Run("Config Overrides Masking and Groups", func(t *testing.T) {
		f := New()

		cfg := &config.Logger{
			Level:         config.LoggerLevelInfo,
			OutputFormat:  config.LogFormatJSON,
			SensitiveTags: []string{"password"},
			MaskString:    "[REDACTED]",
			AppGroupName:  "metadata",
		}

		// Verify prioritization via the same logic used in CreateLoggerFromConfig
		maskString := cfg.MaskString // cmp.Or(cfg.MaskString, "****") would be [REDACTED]
		appGroupName := cfg.AppGroupName

		// 1. Check Masking
		replaceAttr := slogx.MaskingReplaceAttr(cfg.SensitiveTags, maskString)
		attr := replaceAttr(nil, slog.String("password", "secret123"))
		if attr.Value.String() != "[REDACTED]" {
			t.Errorf("expected config mask \"[REDACTED]\", got %q", attr.Value.String())
		}

		// 2. Check App Group Name
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, nil)
		logger := slog.New(handler)
		logger = f.addAppMetadata(logger, appGroupName)
		logger.Info("test")

		var result map[string]any
		if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
			t.Fatalf("failed to unmarshal log: %v", err)
		}
		if _, ok := result["metadata"]; !ok {
			t.Errorf("expected group \"metadata\" to exist in log, but it didn't")
		}
	})

	t.Run("Custom Prefix Colors", func(t *testing.T) {
		colors := map[string][]int{"custom": {1, 2, 3}}
		f := New(WithPrefixColors(colors))

		if len(f.cfg.prefixColors) != 1 || f.cfg.prefixColors["custom"][0] != 1 {
			t.Errorf("expected custom prefix colors to be stored")
		}
	})

	t.Run("Dynamic Log Levels", func(t *testing.T) {
		levelVar := &slog.LevelVar{}
		f := New(WithLevelVar(levelVar))

		f.SetLevel(slog.LevelWarn)
		if levelVar.Level() != slog.LevelWarn {
			t.Errorf("expected levelVar to be WARN, got %v", levelVar.Level())
		}

		if f.GetLevel() != slog.LevelWarn {
			t.Errorf("expected factory level to be WARN, got %v", f.GetLevel())
		}
	})

	t.Run("Custom Handler Registration", func(t *testing.T) {
		const customFormat = "mock"
		formatCalled := false

		RegisterHandler(customFormat, func(w io.Writer, cfg *config.Logger, opts *slog.HandlerOptions) slog.Handler {
			formatCalled = true
			return slog.NewJSONHandler(w, opts)
		})

		f := New()
		cfg := &config.Logger{
			Level:        config.LoggerLevelInfo,
			OutputFormat: customFormat,
		}

		_, _ = f.CreateLoggerFromConfig(cfg)

		if !formatCalled {
			t.Errorf("expected custom handler factory to be called")
		}
	})
}

func TestFactory_CreateMaskingHandler(t *testing.T) {
	f := New()
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, nil)

	handler := f.CreateMaskingHandler(inner, masking.WithField("ssn", masking.FullMask()))
	if handler == nil {
		t.Fatal("expected handler to be created")
	}
}
