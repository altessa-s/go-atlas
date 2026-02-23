// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package masking_test

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/altessa-s/go-atlas/observability/slog/handler/masking"
)

func TestNewHandler_WithDefaultMask(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})

	customMask := func(value string) string { return "REDACTED" }
	h := masking.NewHandler(inner,
		masking.WithDefaultMask(customMask),
		masking.WithField("password", customMask),
	)

	logger := slog.New(h)
	logger.Info("test", "password", "secret123")

	if !bytes.Contains(buf.Bytes(), []byte("REDACTED")) {
		t.Errorf("output = %q, expected REDACTED", buf.String())
	}
}

func TestNewHandler_WithMaskNestedFields(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})

	h := masking.NewHandler(inner,
		masking.WithMaskNestedFields(),
		masking.WithField("secret", nil),
	)

	logger := slog.New(h)
	logger.InfoContext(t.Context(), "test",
		slog.Group("nested", slog.String("secret", "value")),
	)

	output := buf.String()
	if output == "" {
		t.Error("expected output")
	}
}

func TestNewHandler_WithCaseSensitive(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})

	h := masking.NewHandler(inner,
		masking.WithCaseSensitive(),
		masking.WithField("Password", nil),
	)

	logger := slog.New(h)
	logger.Info("test", "Password", "secret")

	output := buf.String()
	if output == "" {
		t.Error("expected output")
	}
}

func TestHandler_WithAttrs_Masking(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})

	h := masking.NewHandler(inner, masking.WithField("token", nil))

	h2 := h.WithAttrs([]slog.Attr{slog.String("token", "abc123")})
	logger := slog.New(h2)
	logger.Info("test")

	output := buf.String()
	if output == "" {
		t.Error("expected output")
	}
}

func TestNewHandler_WithDefaults_Patterns(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})

	h := masking.NewHandler(inner, masking.WithDefaults())
	logger := slog.New(h)
	logger.Info("test", "password", "mysecret", "email", "user@example.com")

	if bytes.Contains(buf.Bytes(), []byte("mysecret")) {
		t.Error("password value should be masked")
	}
}
