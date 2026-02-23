// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"log/slog"
	"testing"
	"time"
)

func TestNewLogger(t *testing.T) {
	l := NewLogger(slog.Default())
	if l == nil {
		t.Fatal("NewLogger returned nil")
	}
}

func TestNewLogger_NilLogger(t *testing.T) {
	l := NewLogger(nil)
	if l == nil {
		t.Fatal("NewLogger(nil) returned nil")
	}
}

func TestNewLogger_WithTimeout(t *testing.T) {
	l := NewLogger(slog.Default(), WithLogTimeout(5*time.Second))
	if l == nil {
		t.Fatal("returned nil")
	}
}

func TestLogger_Methods(t *testing.T) {
	l := NewLogger(slog.New(slog.DiscardHandler))
	// Should not panic
	l.Error("test error", "key", "value")
	l.Warn("test warn", "key", "value")
	l.Info("test info", "key", "value")
	l.Debug("test debug", "key", "value")
}

func TestLogger_NilLoggerMethods(t *testing.T) {
	l := Logger{} // logger field is nil
	// Should not panic
	l.Error("test")
	l.Warn("test")
	l.Info("test")
	l.Debug("test")
}

func TestDefaultLogTimeout(t *testing.T) {
	if DefaultLogTimeout <= 0 {
		t.Fatalf("DefaultLogTimeout = %v", DefaultLogTimeout)
	}
}
