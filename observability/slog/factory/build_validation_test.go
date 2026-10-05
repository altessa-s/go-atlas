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
	"github.com/altessa-s/go-atlas/observability/slog/handler/masking"
)

// isolatedCapture registers a uniquely named capture format and returns the
// config's builder with an isolated level var, so tests can run in parallel
// without touching slogx.GlobalLevel or sharing a buffer.
func isolatedCapture(t *testing.T, cfg *config.Logger) (*factory.LoggerBuilder, *slog.LevelVar, *bytes.Buffer) {
	t.Helper()

	var buf bytes.Buffer
	format := config.LogFormat("capture-" + t.Name())
	factory.RegisterHandler(format, func(_ io.Writer, _ *config.Logger, opts *slog.HandlerOptions) slog.Handler {
		return slog.NewJSONHandler(&buf, opts)
	})
	cfg.OutputFormat = format

	levelVar := &slog.LevelVar{}
	levelVar.Set(slog.Level(42))
	return factory.New(cfg).WithLevelVar(levelVar), levelVar, &buf
}

func decodeRecord(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()

	var record map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &record))
	return record
}

// TestBuild_InvalidMaskRules is the regression for invalid mask rules being
// silently skipped: the error was appended to the builder after Build had
// already checked it, so Build returned a logger and a nil error.
func TestBuild_InvalidMaskRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		rule config.LoggerMaskRule
	}{
		{name: "unknown mask type", rule: config.LoggerMaskRule{Field: "ssn", Type: "no-such-mask"}},
		{name: "params on a parameterless mask", rule: config.LoggerMaskRule{Field: "ssn", Type: "full", Params: map[string]any{"x": 1}}},
		{name: "missing type", rule: config.LoggerMaskRule{Field: "ssn"}},
		{name: "neither field nor pattern", rule: config.LoggerMaskRule{Type: "full"}},
		{name: "invalid pattern regex", rule: config.LoggerMaskRule{Pattern: "(unclosed", Type: "full"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := &config.Logger{
				Level:     config.LoggerLevelInfo,
				MaskRules: []config.LoggerMaskRule{{Field: "token", Type: "full"}, tc.rule},
			}
			b, levelVar, _ := isolatedCapture(t, cfg)

			logger, err := b.Build()
			require.Error(t, err)
			require.ErrorContains(t, err, "invalid mask rule 1")
			require.Nil(t, logger, "no half-initialized logger on error")
			require.Equal(t, slog.Level(42), levelVar.Level(), "a failed Build must not touch the level var")
		})
	}
}

// TestBuild_InvalidMaskRules_AllReported checks that every invalid rule is
// reported, not just the first.
func TestBuild_InvalidMaskRules_AllReported(t *testing.T) {
	t.Parallel()

	cfg := &config.Logger{
		Level: config.LoggerLevelInfo,
		MaskRules: []config.LoggerMaskRule{
			{Field: "a", Type: "no-such-mask"},
			{Pattern: "(", Type: "full"},
		},
	}
	b, _, _ := isolatedCapture(t, cfg)

	_, err := b.Build()
	require.ErrorContains(t, err, "invalid mask rule 0 (a)")
	require.ErrorContains(t, err, "invalid mask rule 1 (()")
}

// TestBuild_ValidMaskRules is the positive control: valid field and pattern
// rules build and mask.
func TestBuild_ValidMaskRules(t *testing.T) {
	t.Parallel()

	cfg := &config.Logger{
		Level: config.LoggerLevelInfo,
		MaskRules: []config.LoggerMaskRule{
			{Field: "ssn", Type: "full"},
			{Pattern: `(?i)^card_.*`, Type: "fixed", Params: map[string]any{"value": "[CARD]"}},
		},
	}
	b, _, buf := isolatedCapture(t, cfg)

	logger, err := b.Build()
	require.NoError(t, err)
	logger.Info("x", "ssn", "123-45-6789", "card_number", "4111111111111111")

	record := decodeRecord(t, buf)
	require.Equal(t, masking.FullMask()("123-45-6789"), record["ssn"])
	require.Equal(t, "[CARD]", record["card_number"])
}

// TestBuild_DefaultMasksCaseInsensitive covers the factory side of
// masking.WithDefaults no longer switching matching to case-sensitive: a
// mixed-case key that only an exact-name default covers used to leak.
func TestBuild_DefaultMasksCaseInsensitive(t *testing.T) {
	t.Parallel()

	const bearer = "Bearer abcdefghijklmnop"

	cfg := &config.Logger{Level: config.LoggerLevelInfo, EnableDefaultMasks: true}
	b, _, buf := isolatedCapture(t, cfg)

	logger, err := b.Build()
	require.NoError(t, err)
	logger.Info("x", "Authorization", bearer)

	require.NotEqual(t, bearer, decodeRecord(t, buf)["Authorization"])
}

// TestBuild_MaskRuleOverridesCaseFoldedDefault checks that an explicit mask
// rule beats the default for a key differing only by case, deterministically.
func TestBuild_MaskRuleOverridesCaseFoldedDefault(t *testing.T) {
	t.Parallel()

	const email = "someone@example.com"

	for range 20 {
		cfg := &config.Logger{
			Level:              config.LoggerLevelInfo,
			EnableDefaultMasks: true,
			MaskRules:          []config.LoggerMaskRule{{Field: "Email", Type: "fixed", Params: map[string]any{"value": "[E]"}}},
		}
		b, _, buf := isolatedCapture(t, cfg)

		logger, err := b.Build()
		require.NoError(t, err)
		logger.Info("x", "email", email)

		require.Equal(t, "[E]", decodeRecord(t, buf)["email"])
	}
}

// TestBuild_InvalidMaskRulesWithLevelNone is the regression for the
// LoggerLevelNone shortcut returning the discard logger before mask rules
// were validated, hiding a broken config until the level was raised.
func TestBuild_InvalidMaskRulesWithLevelNone(t *testing.T) {
	t.Parallel()

	cfg := &config.Logger{
		Level:     config.LoggerLevelNone,
		MaskRules: []config.LoggerMaskRule{{Field: "ssn", Type: "no-such-mask"}},
	}
	b, levelVar, _ := isolatedCapture(t, cfg)

	logger, err := b.Build()
	require.ErrorContains(t, err, "invalid mask rule 0 (ssn)")
	require.Nil(t, logger)
	require.Equal(t, slog.Level(42), levelVar.Level())
}

// TestBuild_LevelNoneDiscards checks that a valid config at level none still
// yields the discard logger.
func TestBuild_LevelNoneDiscards(t *testing.T) {
	t.Parallel()

	cfg := &config.Logger{
		Level:     config.LoggerLevelNone,
		MaskRules: []config.LoggerMaskRule{{Field: "ssn", Type: "full"}},
	}
	b, _, buf := isolatedCapture(t, cfg)

	logger, err := b.Build()
	require.NoError(t, err)
	logger.Error("x")
	require.Zero(t, buf.Len())
}
