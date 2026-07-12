// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package base

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	b := New("mycomp", "middleware", "path", nil)
	require.Equal(t, "mycomp", b.Name())
	require.NotNil(t, b.Logger())
}

func TestNew_WithLogger(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	b := New("test", "interceptor", "method", logger)
	require.Equal(t, logger, b.Logger())
}

func TestNewWithFilter(t *testing.T) {
	b := NewWithFilter("filtered", "middleware", "path", []string{"/health"}, nil, nil)
	require.Equal(t, "filtered", b.Name())
	require.NotNil(t, b.Logger())
}

func TestShouldIgnore(t *testing.T) {
	tests := []struct {
		name      string
		endpoints []string
		patterns  []*regexp.Regexp
		check     string
		want      bool
	}{
		{"exact_match", []string{"/health"}, nil, "/health", true},
		{"exact_match_case_insensitive", []string{"/health"}, nil, "/Health", true},
		{"no_match", []string{"/health"}, nil, "/api", false},
		{"pattern_match", nil, []*regexp.Regexp{regexp.MustCompile(`^/internal`)}, "/internal/debug", true},
		{"pattern_no_match", nil, []*regexp.Regexp{regexp.MustCompile(`^/internal`)}, "/api/v1", false},
		{"no_filter", nil, nil, "/anything", false},
		{"multiple_endpoints", []string{"/health", "/metrics"}, nil, "/metrics", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewWithFilter("test", "middleware", "path", tt.endpoints, tt.patterns, nil)
			got := b.ShouldIgnore(tt.check)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestShouldIgnore_NoFilter(t *testing.T) {
	b := New("test", "interceptor", "method", nil)
	require.False(t, b.ShouldIgnore("/any/method"), "ShouldIgnore should return false without filter")
}

func TestInternEndpoint(t *testing.T) {
	b := New("test", "middleware", "path", nil)
	got := b.InternEndpoint("/test/path")
	require.Equal(t, "/test/path", got)
	// Interning should return the same pointer for the same string.
	got2 := b.InternEndpoint("/test/path")
	require.Equal(t, got2, got)
}

func TestLogMethods_NoPanic(t *testing.T) {
	b := New("test", "middleware", "path", slog.New(slog.DiscardHandler))
	ctx := t.Context()

	b.LogIgnored(ctx, "/health")
	b.LogDebug(ctx, "debug msg", "/api")
	b.LogWarn(ctx, "warn msg", "/api", nil)
	b.LogWarn(ctx, "warn msg", "/api", errors.New("test error"))
	b.LogError(ctx, "error msg", "/api", nil)
	b.LogError(ctx, "error msg", "/api", errors.New("test error"))
}

func TestLogMethods_NilLogger(t *testing.T) {
	b := New("test", "interceptor", "method", nil)
	ctx := t.Context()

	// Should not panic with nil logger input (defaults to DiscardHandler).
	b.LogIgnored(ctx, "/test")
	b.LogDebug(ctx, "msg", "/test")
	b.LogWarn(ctx, "msg", "/test", errors.New("err"))
	b.LogError(ctx, "msg", "/test", errors.New("err"))
}

// logRecord captures a single slog record for test assertions.
type logRecord struct {
	Level   slog.Level
	Message string
	Attrs   map[string]string
}

// captureHandler is a slog.Handler that captures log records for testing.
type captureHandler struct {
	records []logRecord
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	rec := logRecord{
		Level:   r.Level,
		Message: r.Message,
		Attrs:   make(map[string]string),
	}
	r.Attrs(func(a slog.Attr) bool {
		rec.Attrs[a.Key] = a.Value.String()
		return true
	})
	h.records = append(h.records, rec)
	return nil
}

func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }

func TestLogIgnored_Attributes_Middleware(t *testing.T) {
	h := &captureHandler{}
	logger := slog.New(h)
	b := New("cors", "middleware", "path", logger)

	b.LogIgnored(t.Context(), "/health")

	require.Len(t, h.records, 1)
	rec := h.records[0]
	require.Equal(t, slog.LevelDebug, rec.Level)
	require.Equal(t, "ignored", rec.Message)
	assertAttr(t, rec, "middleware", "cors")
	assertAttr(t, rec, "path", "/health")
}

func TestLogIgnored_Attributes_Interceptor(t *testing.T) {
	h := &captureHandler{}
	logger := slog.New(h)
	b := New("auth", "interceptor", "method", logger)

	b.LogIgnored(t.Context(), "/grpc.health.v1.Health/Check")

	require.Len(t, h.records, 1)
	rec := h.records[0]
	assertAttr(t, rec, "interceptor", "auth")
	assertAttr(t, rec, "method", "/grpc.health.v1.Health/Check")
}

func TestLogDebug_Attributes(t *testing.T) {
	h := &captureHandler{}
	logger := slog.New(h)
	b := New("tracing", "middleware", "path", logger)

	b.LogDebug(t.Context(), "request started", "/api/v1", slog.String("trace_id", "abc123"))

	require.Len(t, h.records, 1)
	rec := h.records[0]
	require.Equal(t, slog.LevelDebug, rec.Level)
	require.Equal(t, "request started", rec.Message)
	assertAttr(t, rec, "middleware", "tracing")
	assertAttr(t, rec, "path", "/api/v1")
	assertAttr(t, rec, "trace_id", "abc123")
}

func TestLogWarn_WithError(t *testing.T) {
	h := &captureHandler{}
	logger := slog.New(h)
	b := New("limiter", "interceptor", "method", logger)

	testErr := errors.New("rate exceeded")
	b.LogWarn(t.Context(), "rate limited", "/api.Service/Do", testErr)

	require.Len(t, h.records, 1)
	rec := h.records[0]
	require.Equal(t, slog.LevelWarn, rec.Level)
	assertAttr(t, rec, "interceptor", "limiter")
	assertAttr(t, rec, "method", "/api.Service/Do")
	_, ok := rec.Attrs["error"]
	require.True(t, ok, "expected error attribute to be present")
}

func TestLogWarn_NilError(t *testing.T) {
	h := &captureHandler{}
	logger := slog.New(h)
	b := New("test", "middleware", "path", logger)

	b.LogWarn(t.Context(), "warning", "/test", nil)

	rec := h.records[0]
	_, ok := rec.Attrs["error"]
	require.False(t, ok, "error attribute should not be present when err is nil")
}

func TestLogError_WithError(t *testing.T) {
	h := &captureHandler{}
	logger := slog.New(h)
	b := New("recovery", "middleware", "path", logger)

	testErr := errors.New("panic recovered")
	b.LogError(t.Context(), "recovered", "/api/crash", testErr, slog.String("stack", "..."))

	require.Len(t, h.records, 1)
	rec := h.records[0]
	require.Equal(t, slog.LevelError, rec.Level)
	require.Equal(t, "recovered", rec.Message)
	assertAttr(t, rec, "middleware", "recovery")
	assertAttr(t, rec, "path", "/api/crash")
	assertAttr(t, rec, "stack", "...")
	_, ok := rec.Attrs["error"]
	require.True(t, ok, "expected error attribute")
}

func TestLogError_NilError(t *testing.T) {
	h := &captureHandler{}
	logger := slog.New(h)
	b := New("test", "interceptor", "method", logger)

	b.LogError(t.Context(), "error", "/test", nil)

	rec := h.records[0]
	_, ok := rec.Attrs["error"]
	require.False(t, ok, "error attribute should not be present when err is nil")
}

// levelHandler wraps captureHandler with a minimum-level gate so tests can
// exercise the early-out in the Log* methods.
type levelHandler struct {
	*captureHandler
	min slog.Level
}

func (h levelHandler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.min }

func TestLogMethods_BelowLevel_NotLogged(t *testing.T) {
	c := &captureHandler{}
	logger := slog.New(levelHandler{captureHandler: c, min: slog.LevelInfo})
	b := New("test", "middleware", "path", logger)
	ctx := t.Context()

	b.LogIgnored(ctx, "/health")
	b.LogDebug(ctx, "msg", "/api")
	require.Empty(t, c.records, "debug records must be suppressed below the handler level")

	b.LogWarn(ctx, "warn", "/api", nil)
	require.Len(t, c.records, 1, "warn must still be logged when the handler level allows it")
}

func TestLogDebug_DisabledLevel_NoAllocs(t *testing.T) {
	logger := slog.New(levelHandler{captureHandler: &captureHandler{}, min: slog.LevelInfo})
	b := New("test", "middleware", "path", logger)
	ctx := t.Context()
	attrs := []slog.Attr{slog.String("k", "v")}

	allocs := testing.AllocsPerRun(100, func() {
		b.LogDebug(ctx, "msg", "/api", attrs...)
		b.LogIgnored(ctx, "/api")
	})
	require.Zero(t, allocs, "disabled-level debug logging must not allocate")
}

func assertAttr(t *testing.T, rec logRecord, key, want string) {
	t.Helper()
	got, ok := rec.Attrs[key]
	require.True(t, ok)
	require.Equal(t, want, got)
}
