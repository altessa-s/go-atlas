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
)

func TestNew(t *testing.T) {
	b := New("mycomp", "middleware", "path", nil)
	if b.Name() != "mycomp" {
		t.Fatalf("Name() = %q, want %q", b.Name(), "mycomp")
	}
	if b.Logger() == nil {
		t.Fatal("Logger() should not be nil when constructed with nil")
	}
}

func TestNew_WithLogger(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	b := New("test", "interceptor", "method", logger)
	if b.Logger() != logger {
		t.Fatal("Logger() should return the provided logger")
	}
}

func TestNewWithFilter(t *testing.T) {
	b := NewWithFilter("filtered", "middleware", "path", []string{"/health"}, nil, nil)
	if b.Name() != "filtered" {
		t.Fatalf("Name() = %q, want %q", b.Name(), "filtered")
	}
	if b.Logger() == nil {
		t.Fatal("Logger() should not be nil")
	}
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
			if got := b.ShouldIgnore(tt.check); got != tt.want {
				t.Fatalf("ShouldIgnore(%q) = %v, want %v", tt.check, got, tt.want)
			}
		})
	}
}

func TestShouldIgnore_NoFilter(t *testing.T) {
	b := New("test", "interceptor", "method", nil)
	if b.ShouldIgnore("/any/method") {
		t.Fatal("ShouldIgnore should return false without filter")
	}
}

func TestInternEndpoint(t *testing.T) {
	b := New("test", "middleware", "path", nil)
	got := b.InternEndpoint("/test/path")
	if got != "/test/path" {
		t.Fatalf("InternEndpoint() = %q, want %q", got, "/test/path")
	}
	// Interning should return the same pointer for the same string.
	got2 := b.InternEndpoint("/test/path")
	if got != got2 {
		t.Fatal("InternEndpoint should return the same interned string")
	}
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

	if len(h.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(h.records))
	}
	rec := h.records[0]
	if rec.Level != slog.LevelDebug {
		t.Fatalf("level = %v, want Debug", rec.Level)
	}
	if rec.Message != "ignored" {
		t.Fatalf("message = %q, want %q", rec.Message, "ignored")
	}
	assertAttr(t, rec, "middleware", "cors")
	assertAttr(t, rec, "path", "/health")
}

func TestLogIgnored_Attributes_Interceptor(t *testing.T) {
	h := &captureHandler{}
	logger := slog.New(h)
	b := New("auth", "interceptor", "method", logger)

	b.LogIgnored(t.Context(), "/grpc.health.v1.Health/Check")

	if len(h.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(h.records))
	}
	rec := h.records[0]
	assertAttr(t, rec, "interceptor", "auth")
	assertAttr(t, rec, "method", "/grpc.health.v1.Health/Check")
}

func TestLogDebug_Attributes(t *testing.T) {
	h := &captureHandler{}
	logger := slog.New(h)
	b := New("tracing", "middleware", "path", logger)

	b.LogDebug(t.Context(), "request started", "/api/v1", slog.String("trace_id", "abc123"))

	if len(h.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(h.records))
	}
	rec := h.records[0]
	if rec.Level != slog.LevelDebug {
		t.Fatalf("level = %v, want Debug", rec.Level)
	}
	if rec.Message != "request started" {
		t.Fatalf("message = %q, want %q", rec.Message, "request started")
	}
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

	if len(h.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(h.records))
	}
	rec := h.records[0]
	if rec.Level != slog.LevelWarn {
		t.Fatalf("level = %v, want Warn", rec.Level)
	}
	assertAttr(t, rec, "interceptor", "limiter")
	assertAttr(t, rec, "method", "/api.Service/Do")
	if _, ok := rec.Attrs["error"]; !ok {
		t.Fatal("expected error attribute to be present")
	}
}

func TestLogWarn_NilError(t *testing.T) {
	h := &captureHandler{}
	logger := slog.New(h)
	b := New("test", "middleware", "path", logger)

	b.LogWarn(t.Context(), "warning", "/test", nil)

	rec := h.records[0]
	if _, ok := rec.Attrs["error"]; ok {
		t.Fatal("error attribute should not be present when err is nil")
	}
}

func TestLogError_WithError(t *testing.T) {
	h := &captureHandler{}
	logger := slog.New(h)
	b := New("recovery", "middleware", "path", logger)

	testErr := errors.New("panic recovered")
	b.LogError(t.Context(), "recovered", "/api/crash", testErr, slog.String("stack", "..."))

	if len(h.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(h.records))
	}
	rec := h.records[0]
	if rec.Level != slog.LevelError {
		t.Fatalf("level = %v, want Error", rec.Level)
	}
	if rec.Message != "recovered" {
		t.Fatalf("message = %q, want %q", rec.Message, "recovered")
	}
	assertAttr(t, rec, "middleware", "recovery")
	assertAttr(t, rec, "path", "/api/crash")
	assertAttr(t, rec, "stack", "...")
	if _, ok := rec.Attrs["error"]; !ok {
		t.Fatal("expected error attribute")
	}
}

func TestLogError_NilError(t *testing.T) {
	h := &captureHandler{}
	logger := slog.New(h)
	b := New("test", "interceptor", "method", logger)

	b.LogError(t.Context(), "error", "/test", nil)

	rec := h.records[0]
	if _, ok := rec.Attrs["error"]; ok {
		t.Fatal("error attribute should not be present when err is nil")
	}
}

func assertAttr(t *testing.T, rec logRecord, key, want string) {
	t.Helper()
	got, ok := rec.Attrs[key]
	if !ok {
		t.Fatalf("attribute %q not found in log record (attrs: %v)", key, rec.Attrs)
	}
	if got != want {
		t.Fatalf("attribute %q = %q, want %q", key, got, want)
	}
}
