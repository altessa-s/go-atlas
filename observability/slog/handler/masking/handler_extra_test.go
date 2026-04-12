// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package masking_test

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/slog/handler/masking"
)

func TestNewHandler_WithField(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	h := masking.NewHandler(inner,
		masking.WithField("password", masking.FullMask()),
	)

	logger := slog.New(h)
	logger.Info("test", "password", "secret123", "name", "alice")

	output := buf.String()
	require.NotEmpty(t, output)
}

func TestNewHandler_WithDefaults(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	h := masking.NewHandler(inner, masking.WithDefaults())

	logger := slog.New(h)
	logger.Info("test", "password", "secret", "api_key", "key123")

	output := buf.String()
	require.NotEmpty(t, output)
}

func TestHandler_Enabled(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	h := masking.NewHandler(inner)

	require.False(t, h.Enabled(t.Context(), slog.LevelInfo), "Enabled(Info) should be false when inner handler is Warn level")
	require.True(t, h.Enabled(t.Context(), slog.LevelWarn), "Enabled(Warn) should be true")
}

func TestHandler_WithAttrs(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	h := masking.NewHandler(inner, masking.WithField("secret", masking.FullMask()))

	h2 := h.WithAttrs([]slog.Attr{slog.String("extra", "val")})
	require.NotNil(t, h2)
}

func TestHandler_WithGroup(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	h := masking.NewHandler(inner)

	h2 := h.WithGroup("mygroup")
	require.NotNil(t, h2)
}
