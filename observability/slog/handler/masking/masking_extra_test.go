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

// TestHandler_KeyResolution pins case folding, nested-path precedence and
// path-based patterns across the bare-key and full-path lookup modes.
func TestHandler_KeyResolution(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts []masking.Option
		attr slog.Attr
		want []string
		deny []string
	}{
		{
			name: "unicode key folds like strings.ToLower",
			opts: []masking.Option{masking.WithField("Κλειδί", masking.FixedMask("MASKED"))},
			attr: slog.String("ΚΛΕΙΔΊ", "v1"),
			want: []string{"MASKED"},
			deny: []string{"v1"},
		},
		{
			name: "mixed-case ascii key inside nested group",
			opts: []masking.Option{masking.WithMaskNestedFields(), masking.WithField("password", masking.FixedMask("MASKED"))},
			attr: slog.Group("Request", slog.Group("Auth", slog.String("PassWord", "v1"), slog.String("User", "alice"))),
			want: []string{"MASKED", "alice"},
			deny: []string{"v1"},
		},
		{
			name: "dotted rule wins over bare key",
			opts: []masking.Option{
				masking.WithMaskNestedFields(),
				masking.WithField("token", masking.FixedMask("FLAT")),
				masking.WithField("Req.Token", masking.FixedMask("NESTED")),
			},
			attr: slog.Group("req", slog.String("TOKEN", "v1"), slog.Group("other", slog.String("token", "v2"))),
			want: []string{"NESTED", "FLAT"},
			deny: []string{"v1", "v2"},
		},
		{
			name: "pattern matches full path",
			opts: []masking.Option{masking.WithMaskNestedFields(), masking.WithPattern(`^req\.id$`, masking.FixedMask("MASKED"))},
			attr: slog.Group("req", slog.String("id", "v1")),
			want: []string{"MASKED"},
			deny: []string{"v1"},
		},
		{
			name: "dotted rule ignored without nested masking",
			opts: []masking.Option{masking.WithField("req.id", masking.FixedMask("MASKED"))},
			attr: slog.Group("req", slog.String("id", "v1")),
			want: []string{"v1"},
			deny: []string{"MASKED"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			logger := slog.New(masking.NewHandler(slog.NewTextHandler(&buf, nil), tc.opts...))
			logger.LogAttrs(t.Context(), slog.LevelInfo, "msg", tc.attr)

			out := buf.String()
			for _, s := range tc.want {
				require.Contains(t, out, s)
			}
			for _, s := range tc.deny {
				require.NotContains(t, out, s)
			}
		})
	}
}
