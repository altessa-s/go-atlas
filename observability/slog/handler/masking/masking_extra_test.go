// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package masking_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

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

	require.Contains(t, buf.String(), "REDACTED")
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

	require.NotEmpty(t, buf.String())
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

	require.NotEmpty(t, buf.String())
}

func TestHandler_WithAttrs_Masking(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})

	h := masking.NewHandler(inner, masking.WithField("token", nil))

	h2 := h.WithAttrs([]slog.Attr{slog.String("token", "abc123")})
	logger := slog.New(h2)
	logger.Info("test")

	require.NotEmpty(t, buf.String())
}

func TestNewHandler_WithDefaults_Patterns(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})

	h := masking.NewHandler(inner, masking.WithDefaults())
	logger := slog.New(h)
	logger.Info("test", "password", "mysecret", "email", "user@example.com")

	require.NotContains(t, buf.String(), "mysecret", "password value should be masked")
}

// TestHandler_DepthLimitDoesNotLeak is a regression for a leak a fuzz target
// found: at the descent limit the walker reported "not rebuilt", and the caller
// passed the whole value to the underlying handler — which rendered every field
// it contained, masked names included.
//
// A self-referencing struct reaches the limit immediately, but so does any
// legitimately deep object graph; the guard against runaway recursion was
// printing exactly what it was guarding.
func TestHandler_DepthLimitDoesNotLeak(t *testing.T) {
	t.Parallel()

	type node struct {
		User     string
		Password string
		Inner    *node
	}

	const secret = "s3cr3t-value-that-must-not-appear"

	cyclic := &node{User: "svc", Password: secret}
	cyclic.Inner = cyclic

	var sb strings.Builder
	inner := slog.NewTextHandler(&sb, &slog.HandlerOptions{
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})

	logger := slog.New(masking.NewHandler(inner,
		masking.WithDefaults(),
		masking.WithMaskNestedFields(),
		masking.WithField("password", masking.FullMask()),
	))
	logger.LogAttrs(t.Context(), slog.LevelInfo, "msg", slog.Any("account", cyclic))

	out := sb.String()
	require.NotContains(t, out, secret, "the masked value survived the depth cutoff:\n%s", out)
	require.NotContains(t, out, "Password:", "a struct was rendered raw at the depth cutoff:\n%s", out)
}
