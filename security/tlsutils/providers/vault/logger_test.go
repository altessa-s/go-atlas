// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlsvault_test

import (
	"log/slog"
	"testing"

	tlsvault "github.com/altessa-s/go-atlas/security/tlsutils/providers/vault"
)

func TestNewLogger(t *testing.T) {
	l := tlsvault.NewLogger(nil)
	if l == nil {
		t.Fatal("NewLogger(nil) returned nil")
	}
}

func TestNewLogger_WithSlog(t *testing.T) {
	l := tlsvault.NewLogger(slog.Default())
	if l == nil {
		t.Fatal("NewLogger(slog.Default()) returned nil")
	}
}

func TestLogger_Methods(t *testing.T) {
	l := tlsvault.NewLogger(slog.New(slog.DiscardHandler))

	// These should not panic
	l.Trace("trace message")
	l.Debug("debug message")
	l.Info("info message")
	l.Warn("warn message")
	l.Error("error message")
}

func TestLogger_MethodsWithFields(t *testing.T) {
	l := tlsvault.NewLogger(slog.New(slog.DiscardHandler))

	fields := map[string]any{"key": "value", "num": 42}

	// These should not panic
	l.Trace("trace", fields)
	l.Debug("debug", fields)
	l.Info("info", fields)
	l.Warn("warn", fields)
	l.Error("error", fields)
}

func TestNewLoggerWithContext(t *testing.T) {
	l := tlsvault.NewLoggerWithContext(t.Context(), nil)
	if l == nil {
		t.Fatal("NewLoggerWithContext() returned nil")
	}
}

func TestNewLoggerWithContext_NilLogger(t *testing.T) {
	l := tlsvault.NewLoggerWithContext(t.Context(), slog.Default())
	if l == nil {
		t.Fatal("NewLoggerWithContext() returned nil")
	}
}
