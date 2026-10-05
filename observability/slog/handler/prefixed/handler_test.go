// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package prefixed_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/slog/handler/prefixed"
)

const testKey = "module"

// newJSONLogger returns a logger whose prefixed handler writes JSON (with the
// time attribute removed) to the returned buffer.
func newJSONLogger(t *testing.T, opts ...prefixed.Option) (*slog.Logger, *bytes.Buffer) {
	t.Helper()

	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) == 0 && a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})
	opts = append([]prefixed.Option{
		prefixed.WithPrefix(testKey),
		prefixed.WithPrefixFormatter(prefixed.JsonFormatter),
	}, opts...)
	return slog.New(prefixed.NewHandler(inner, opts...)), &buf
}

// decode parses the single JSON record in buf.
func decode(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()

	var record map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &record))
	return record
}

func TestHandler_Prefixes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		log  func(l *slog.Logger)
		want map[string]any
	}{
		{
			name: "record prefix",
			log:  func(l *slog.Logger) { l.Info("m", testKey, "api", "k", "v") },
			want: map[string]any{"level": "INFO", "msg": "m", "k": "v", testKey: "api"},
		},
		{
			name: "no prefix passes through",
			log:  func(l *slog.Logger) { l.Info("m", "k", "v") },
			want: map[string]any{"level": "INFO", "msg": "m", "k": "v"},
		},
		{
			name: "with attrs and record prefixes merge in order",
			log: func(l *slog.Logger) {
				l.With(testKey, "api").With(testKey, "server", "k", "v").Info("m", testKey, "v1")
			},
			want: map[string]any{"level": "INFO", "msg": "m", "k": "v", testKey: "api:server:v1"},
		},
		{
			name: "non-string prefix key is left untouched",
			log:  func(l *slog.Logger) { l.Info("m", testKey, 7) },
			want: map[string]any{"level": "INFO", "msg": "m", testKey: float64(7)},
		},
		{
			name: "prefix stays at the root after a group",
			log: func(l *slog.Logger) {
				l.With(testKey, "api").WithGroup("req").Info("m", "id", 1)
			},
			want: map[string]any{
				"level": "INFO", "msg": "m", testKey: "api",
				"req": map[string]any{"id": float64(1)},
			},
		},
		{
			name: "prefix-only logger with an empty group",
			log: func(l *slog.Logger) {
				l.With(testKey, "api").WithGroup("req").Info("m")
			},
			want: map[string]any{"level": "INFO", "msg": "m", testKey: "api"},
		},
		{
			name: "each prefix stays in the group it was attached to",
			log: func(l *slog.Logger) {
				l.With(testKey, "api", "a", 0).
					WithGroup("req").With(testKey, "auth", "b", 1).
					WithGroup("db").Info("m", testKey, "sql", "c", 2)
			},
			want: map[string]any{
				"level": "INFO", "msg": "m", "a": float64(0), testKey: "api",
				"req": map[string]any{
					"b": float64(1), testKey: "auth",
					"db": map[string]any{"c": float64(2), testKey: "sql"},
				},
			},
		},
		{
			name: "record prefix inside a group lands in that group",
			log: func(l *slog.Logger) {
				l.WithGroup("req").Info("m", testKey, "auth")
			},
			want: map[string]any{
				"level": "INFO", "msg": "m",
				"req": map[string]any{testKey: "auth"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			logger, buf := newJSONLogger(t)
			tc.log(logger)
			require.Equal(t, tc.want, decode(t, buf))
		})
	}
}

func TestHandler_GroupedPrefixIsEmittedOnce(t *testing.T) {
	t.Parallel()

	logger, buf := newJSONLogger(t)
	logger.With(testKey, "api").WithGroup("req").Info("m", "id", 1)

	require.Equal(t, 1, strings.Count(buf.String(), `"`+testKey+`"`))
}

// TestHandler_NonStringPrefixKeyKept is the regression for non-string
// attributes under the prefix key being dropped whenever a string prefix was
// extracted from the same attribute list.
func TestHandler_NonStringPrefixKeyKept(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		log  func(l *slog.Logger)
		want string
	}{
		{
			name: "record attrs",
			log:  func(l *slog.Logger) { l.Info("m", testKey, "api", slog.Group(testKey, "id", 7)) },
			want: `{"level":"INFO","msg":"m","module":{"id":7},"module":"api"}` + "\n",
		},
		{
			name: "with attrs",
			log:  func(l *slog.Logger) { l.With(testKey, "api", slog.Group(testKey, "id", 7)).Info("m") },
			want: `{"level":"INFO","msg":"m","module":{"id":7},"module":"api"}` + "\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			logger, buf := newJSONLogger(t)
			tc.log(logger)
			require.Equal(t, tc.want, buf.String())
		})
	}
}

func TestHandler_TextOutputWithGroups(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) == 0 && a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})
	logger := slog.New(prefixed.NewHandler(inner, prefixed.WithPrefix(testKey)))

	logger.With(testKey, "api").With(testKey, "server").WithGroup("req").Info("m", "id", 1)

	require.Equal(t, `level=INFO msg=m module=[api:server] req.id=1`+"\n", buf.String())
}

func TestHandler_Delimiter(t *testing.T) {
	t.Parallel()

	logger, buf := newJSONLogger(t, prefixed.WithPrefixesDelimiter("/"))
	logger.With(testKey, "api").Info("m", testKey, "v1")

	require.Equal(t, "api/v1", decode(t, buf)[testKey])
}

func TestHandler_NilFormatterResult(t *testing.T) {
	t.Parallel()

	nilFormatter := func([]slog.Value, string) *slog.Value { return nil }
	logger, buf := newJSONLogger(t, prefixed.WithPrefixFormatter(nilFormatter))
	logger.With(testKey, "api").WithGroup("req").Info("m", "id", 1)

	require.Equal(t, map[string]any{
		"level": "INFO", "msg": "m",
		"req": map[string]any{"id": float64(1)},
	}, decode(t, buf))
}

func TestHandler_WithGroupEmptyNameReturnsReceiver(t *testing.T) {
	t.Parallel()

	h := prefixed.NewHandler(slog.DiscardHandler, prefixed.WithPrefix(testKey))
	require.Same(t, h, h.WithGroup(""))
}
