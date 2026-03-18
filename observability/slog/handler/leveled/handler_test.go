// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package leveled

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	slogx "github.com/altessa-s/go-atlas/observability/slog"
)

func newTestLogger(buf *bytes.Buffer, opts ...Option) *slog.Logger {
	base := slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	h := NewHandler(base, opts...)
	return slog.New(h)
}

func TestEnabled_WithSubsystem(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf,
		WithDefaultLevel(slog.LevelInfo),
		WithSubsystemLevels(map[string]slog.Level{
			"scheduler": slog.LevelDebug,
			"grpc":      slog.LevelWarn,
		}),
	)

	// Capture subsystem via With
	schedLogger := logger.With(slogx.Module("scheduler"))
	grpcLogger := logger.With(slogx.Module("grpc"))

	h := schedLogger.Handler()
	assert.True(t, h.Enabled(context.Background(), slog.LevelDebug), "scheduler should allow debug")

	h = grpcLogger.Handler()
	assert.False(t, h.Enabled(context.Background(), slog.LevelInfo), "grpc should reject info")
	assert.True(t, h.Enabled(context.Background(), slog.LevelWarn), "grpc should allow warn")
}

func TestEnabled_WithoutSubsystem(t *testing.T) {
	base := slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelDebug})
	h := NewHandler(base,
		WithDefaultLevel(slog.LevelInfo),
		WithSubsystemLevels(map[string]slog.Level{
			"scheduler": slog.LevelDebug,
		}),
	)

	// Without captured subsystem, delegates to base (which has LevelDebug)
	assert.True(t, h.Enabled(context.Background(), slog.LevelDebug),
		"without subsystem, should delegate to base handler")
}

func TestHandle_FiltersBySubsystemInRecord(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf,
		WithDefaultLevel(slog.LevelInfo),
		WithSubsystemLevels(map[string]slog.Level{
			"scheduler": slog.LevelDebug,
			"grpc":      slog.LevelWarn,
		}),
	)

	// Debug log with scheduler subsystem in record attrs — should pass
	buf.Reset()
	logger.Debug("tick", slogx.Module("scheduler"))
	assert.Contains(t, buf.String(), "tick", "scheduler debug should pass")

	// Debug log with grpc subsystem — should be filtered
	buf.Reset()
	logger.Debug("connect", slogx.Module("grpc"))
	assert.Empty(t, buf.String(), "grpc debug should be filtered")

	// Warn log with grpc subsystem — should pass
	buf.Reset()
	logger.Warn("timeout", slogx.Module("grpc"))
	assert.Contains(t, buf.String(), "timeout", "grpc warn should pass")
}

func TestHandle_FallbackToDefaultLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf,
		WithDefaultLevel(slog.LevelInfo),
		WithSubsystemLevels(map[string]slog.Level{
			"scheduler": slog.LevelDebug,
		}),
	)

	// Unknown subsystem should use default (Info)
	buf.Reset()
	logger.Debug("something", slogx.Module("unknown"))
	assert.Empty(t, buf.String(), "unknown subsystem debug should be filtered by default level")

	buf.Reset()
	logger.Info("something", slogx.Module("unknown"))
	assert.Contains(t, buf.String(), "something", "unknown subsystem info should pass")
}

func TestHandle_NoSubsystemAttribute(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf,
		WithDefaultLevel(slog.LevelInfo),
		WithSubsystemLevels(map[string]slog.Level{
			"scheduler": slog.LevelDebug,
		}),
	)

	// No subsystem attr — uses default level
	buf.Reset()
	logger.Debug("bare debug")
	assert.Empty(t, buf.String(), "bare debug should be filtered by default level")

	buf.Reset()
	logger.Info("bare info")
	assert.Contains(t, buf.String(), "bare info", "bare info should pass")
}

func TestHandle_PassthroughWithCapturedSubsystem(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf,
		WithDefaultLevel(slog.LevelInfo),
		WithSubsystemLevels(map[string]slog.Level{
			"scheduler": slog.LevelDebug,
		}),
	)

	schedLogger := logger.With(slogx.Module("scheduler"))

	buf.Reset()
	schedLogger.Debug("tick")
	assert.Contains(t, buf.String(), "tick", "captured scheduler should pass debug")
}

func TestWithAttrs_CapturesSubsystem(t *testing.T) {
	base := slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelDebug})
	h := NewHandler(base,
		WithDefaultLevel(slog.LevelInfo),
		WithSubsystemLevels(map[string]slog.Level{
			"scheduler": slog.LevelDebug,
		}),
	)

	h2 := h.WithAttrs([]slog.Attr{slogx.Module("scheduler")})
	lh := h2.(*Handler)
	assert.True(t, lh.hasSubsys)
	assert.Equal(t, "scheduler", lh.subsystem)
}

func TestWithAttrs_NoSubsystem(t *testing.T) {
	base := slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelDebug})
	h := NewHandler(base,
		WithDefaultLevel(slog.LevelInfo),
	)

	h2 := h.WithAttrs([]slog.Attr{slog.String("key", "value")})
	lh := h2.(*Handler)
	assert.False(t, lh.hasSubsys)
	assert.Empty(t, lh.subsystem)
}

func TestWithAttrs_PreservesExistingSubsystem(t *testing.T) {
	base := slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelDebug})
	h := NewHandler(base,
		WithDefaultLevel(slog.LevelInfo),
		WithSubsystemLevels(map[string]slog.Level{
			"scheduler": slog.LevelDebug,
			"grpc":      slog.LevelWarn,
		}),
	)

	// First WithAttrs captures "scheduler"
	h2 := h.WithAttrs([]slog.Attr{slogx.Module("scheduler")})
	// Second WithAttrs with another subsystem should keep the original
	h3 := h2.WithAttrs([]slog.Attr{slogx.Module("grpc")})
	lh := h3.(*Handler)
	assert.True(t, lh.hasSubsys)
	assert.Equal(t, "scheduler", lh.subsystem, "should keep first captured subsystem")
}

func TestWithGroup(t *testing.T) {
	base := slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelDebug})
	h := NewHandler(base,
		WithDefaultLevel(slog.LevelInfo),
		WithSubsystemLevels(map[string]slog.Level{
			"scheduler": slog.LevelDebug,
		}),
	)

	// Set subsystem first
	h2 := h.WithAttrs([]slog.Attr{slogx.Module("scheduler")}).WithGroup("details")
	lh := h2.(*Handler)
	assert.True(t, lh.hasSubsys)
	assert.Equal(t, "scheduler", lh.subsystem)
}

func TestWithGroup_EmptyName(t *testing.T) {
	base := slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelDebug})
	h := NewHandler(base, WithDefaultLevel(slog.LevelInfo))

	h2 := h.WithGroup("")
	assert.Same(t, h, h2, "empty group name should return same handler")
}

func TestEmptySubsystemLevels(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf,
		WithDefaultLevel(slog.LevelInfo),
		WithSubsystemLevels(map[string]slog.Level{}),
	)

	// Empty subsystem levels — everything uses default
	buf.Reset()
	logger.Debug("debug msg", slogx.Module("scheduler"))
	assert.Empty(t, buf.String(), "should filter debug with empty subsystem map")

	buf.Reset()
	logger.Info("info msg", slogx.Module("scheduler"))
	assert.Contains(t, buf.String(), "info msg", "should pass info with empty subsystem map")
}

func TestMultipleSubsystemAttrs_TakesFirst(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf,
		WithDefaultLevel(slog.LevelInfo),
		WithSubsystemLevels(map[string]slog.Level{
			"scheduler": slog.LevelDebug,
			"grpc":      slog.LevelError,
		}),
	)

	// Two subsystem attrs in record — first wins ("scheduler" = debug, so debug passes)
	buf.Reset()
	logger.Debug("multi", slogx.Module("scheduler"), slogx.Module("grpc"))
	assert.Contains(t, buf.String(), "multi", "first subsystem (scheduler/debug) should win")
}

func TestCustomSubsystemKey(t *testing.T) {
	var buf bytes.Buffer
	base := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	h := NewHandler(base,
		WithDefaultLevel(slog.LevelInfo),
		WithSubsystemLevels(map[string]slog.Level{
			"mymod": slog.LevelDebug,
		}),
		WithSubsystemKey("module"),
	)
	logger := slog.New(h)

	logger.Debug("custom key", slog.String("module", "mymod"))
	assert.Contains(t, buf.String(), "custom key", "custom subsystem key should work")
}

func TestSubsystemAttributeNotRemoved(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf,
		WithDefaultLevel(slog.LevelInfo),
		WithSubsystemLevels(map[string]slog.Level{
			"scheduler": slog.LevelDebug,
		}),
	)

	buf.Reset()
	logger.Debug("tick", slogx.Module("scheduler"))
	output := buf.String()
	assert.Contains(t, output, "subsystem", "subsystem attribute should be preserved for downstream handlers")
}

func TestIntegration_SubsystemFiltering(t *testing.T) {
	subsystemLevels := map[string]slog.Level{
		"scheduler": slog.LevelDebug,
		"grpc":      slog.LevelWarn,
		"http":      slog.LevelError,
	}

	tests := []struct {
		name      string
		subsystem string
		level     slog.Level
		msg       string
		want      bool
	}{
		{"scheduler debug passes", "scheduler", slog.LevelDebug, "sched-debug", true},
		{"scheduler info passes", "scheduler", slog.LevelInfo, "sched-info", true},
		{"grpc debug filtered", "grpc", slog.LevelDebug, "grpc-debug", false},
		{"grpc info filtered", "grpc", slog.LevelInfo, "grpc-info", false},
		{"grpc warn passes", "grpc", slog.LevelWarn, "grpc-warn", true},
		{"http debug filtered", "http", slog.LevelDebug, "http-debug", false},
		{"http warn filtered", "http", slog.LevelWarn, "http-warn", false},
		{"http error passes", "http", slog.LevelError, "http-error", true},
		{"unknown debug filtered", "unknown", slog.LevelDebug, "unknown-debug", false},
		{"unknown info passes", "unknown", slog.LevelInfo, "unknown-info", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := newTestLogger(&buf,
				WithDefaultLevel(slog.LevelInfo),
				WithSubsystemLevels(subsystemLevels),
			)
			logger.Log(context.Background(), tt.level, tt.msg, slogx.Module(tt.subsystem))
			if tt.want {
				require.Contains(t, buf.String(), tt.msg)
			} else {
				require.Empty(t, buf.String())
			}
		})
	}
}

func TestIntegration_WithLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf,
		WithDefaultLevel(slog.LevelInfo),
		WithSubsystemLevels(map[string]slog.Level{
			"scheduler": slog.LevelDebug,
		}),
	)

	schedLogger := logger.With(slogx.Module("scheduler"))
	otherLogger := logger.With(slogx.Module("other"))

	// Scheduler logger at debug — should pass
	buf.Reset()
	schedLogger.Debug("scheduler tick")
	assert.Contains(t, buf.String(), "scheduler tick")

	// Other logger at debug — should be filtered
	buf.Reset()
	otherLogger.Debug("other debug")
	assert.Empty(t, buf.String())

	// Other logger at info — should pass
	buf.Reset()
	otherLogger.Info("other info")
	assert.Contains(t, buf.String(), "other info")
}

func TestIntegration_SubsystemNotLeakedBetweenRecords(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf,
		WithDefaultLevel(slog.LevelWarn),
		WithSubsystemLevels(map[string]slog.Level{
			"scheduler": slog.LevelDebug,
		}),
	)

	// First record with scheduler — should pass
	buf.Reset()
	logger.Debug("tick", slogx.Module("scheduler"))
	assert.Contains(t, buf.String(), "tick")

	// Second record without subsystem — should be filtered by default level (warn)
	buf.Reset()
	logger.Debug("bare")
	assert.Empty(t, buf.String())
}

func TestLogOutput_ContainsAllAttributes(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf,
		WithDefaultLevel(slog.LevelDebug),
	)

	logger.Info("test", slog.String("key", "value"), slogx.Module("mymod"))
	output := buf.String()
	assert.Contains(t, output, "key=value")
	assert.Contains(t, output, "subsystem=mymod")
}

func TestHandler_EnsuresCorrectChainOrder(t *testing.T) {
	// Verify that the leveled handler preserves attributes for downstream handlers
	var buf bytes.Buffer
	base := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	h := NewHandler(base,
		WithDefaultLevel(slog.LevelInfo),
		WithSubsystemLevels(map[string]slog.Level{
			"scheduler": slog.LevelDebug,
		}),
	)
	logger := slog.New(h)

	schedLogger := logger.With(slogx.Module("scheduler"))
	schedLogger.Debug("check attrs", slog.String("extra", "data"))
	output := buf.String()
	assert.Contains(t, output, "extra=data", "downstream handler should see all attributes")
	assert.Contains(t, output, "subsystem=scheduler", "subsystem attr should be passed through")
}
