// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/observability/slog/factory"
)

// captureLogger builds a JSON logger that writes into buf using the given
// config. It bypasses the factory's auto-detected stdout writer by relying
// on the JSON handler's deterministic format and intercepting output via a
// custom handler registration.
func captureLogger(t *testing.T, cfg *config.Logger) (*slog.Logger, *bytes.Buffer) {
	t.Helper()

	const captureFormat = "auto-masking-test-capture"

	var buf bytes.Buffer
	factory.RegisterHandler(captureFormat, func(_ io.Writer, _ *config.Logger, opts *slog.HandlerOptions) slog.Handler {
		return slog.NewJSONHandler(&buf, opts)
	})

	cfg.OutputFormat = captureFormat
	logger, err := factory.New(cfg).Build()
	require.NoError(t, err)
	return logger, &buf
}

// TestBuilder_MaskingAutoEnabledFromConfig checks the regression: declaring
// any sensitive tags in YAML must auto-enable the advanced masking handler
// — previously SensitiveTags alone gave only the weaker ReplaceAttr-based
// substitution and missed default fields like `email` and pattern matches
// like `*api_key*`.
func TestBuilder_MaskingAutoEnabledFromConfig(t *testing.T) {
	cfg := &config.Logger{
		Level:         config.LoggerLevelInfo,
		SensitiveTags: []string{"password"},
		MaskString:    "[REDACTED]",
	}

	logger, buf := captureLogger(t, cfg)
	logger.Info("auth attempt",
		slog.String("password", "secret-pw-value"),
		slog.String("email", "user@example.com"),
		slog.String("user_api_key", "ak-live-deadbeef"),
	)

	var record map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &record))

	require.NotEqual(t, "secret-pw-value", record["password"],
		"explicit SensitiveTags entry must be masked")
	require.NotContains(t, record["email"], "user@example.com",
		"email must be masked by the wrapper's default rules")
	require.NotContains(t, record["user_api_key"], "ak-live-deadbeef",
		"`*_key` pattern must be masked by the wrapper's default rules")
}

// TestBuilder_MaskingDisabledByDefault is the negative side of the
// regression: with neither WithEnableMasking nor SensitiveTags set, the
// advanced wrapper stays off so we don't accidentally mask user-named
// fields like `email` in callers that opted out of masking entirely.
func TestBuilder_MaskingDisabledByDefault(t *testing.T) {
	cfg := &config.Logger{
		Level: config.LoggerLevelInfo,
	}

	logger, buf := captureLogger(t, cfg)
	logger.Info("notification sent", slog.String("email", "user@example.com"))

	var record map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &record))

	require.Equal(t, "user@example.com", record["email"],
		"without SensitiveTags or WithEnableMasking the wrapper must stay off")
}

// TestBuilder_MaskingEnabledViaCodeOption keeps the existing opt-in path
// honest: WithEnableMasking() alone (no SensitiveTags in config) still
// installs the wrapper using the curated default field set.
func TestBuilder_MaskingEnabledViaCodeOption(t *testing.T) {
	cfg := &config.Logger{Level: config.LoggerLevelInfo}

	const captureFormat = "auto-masking-code-capture"

	var buf bytes.Buffer
	factory.RegisterHandler(captureFormat, func(_ io.Writer, _ *config.Logger, opts *slog.HandlerOptions) slog.Handler {
		return slog.NewJSONHandler(&buf, opts)
	})

	cfg.OutputFormat = captureFormat
	logger, err := factory.New(cfg).WithEnableMasking().Build()
	require.NoError(t, err)

	logger.Info("auth attempt", slog.String("email", "user@example.com"))

	var record map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &record))

	got, _ := record["email"].(string)
	require.NotEqual(t, "user@example.com", got)
	require.Contains(t, got, "*", "default field set should mask email")
}
