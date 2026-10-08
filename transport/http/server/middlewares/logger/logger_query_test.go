// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	slogx "github.com/altessa-s/go-atlas/observability/slog"
)

// queryField runs one request through the middleware and returns the
// http.query field, or "" when it is absent.
func queryField(t *testing.T, target string, opts ...Option) string {
	t.Helper()
	var got string
	lh := LogHandlerFunc(func(_ context.Context, _ string, _ int, fields slogx.Fields) {
		for _, f := range fields {
			if f.Key == FieldKeyHTTPQuery {
				got, _ = f.Value.(string)
			}
		}
	})
	h := Middleware(lh, opts...)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, target, nil))
	return got
}

func TestLogQuery(t *testing.T) {
	t.Parallel()

	redact := WithRedactQueryParams("access_token", "code")
	tests := []struct {
		name   string
		target string
		opts   []Option
		want   string
	}{
		{"off_by_default", "/cb?code=abc", nil, ""},
		{"raw", "/cb?a=1&b=2", []Option{WithLogQuery()}, "a=1&b=2"},
		{"no_query", "/cb", []Option{WithLogQuery()}, ""},
		{"redacted_in_order", "/cb?state=s&code=abc&x=1&code=def", []Option{WithLogQuery(), redact},
			"state=s&code=REDACTED&x=1&code=REDACTED"},
		{"encoded_name", "/cb?acc%65ss_token=t&b=2", []Option{WithLogQuery(), redact}, "acc%65ss_token=REDACTED&b=2"},
		{"name_without_value", "/cb?code&b", []Option{WithLogQuery(), redact}, "code=REDACTED&b"},
		{"case_sensitive", "/cb?Code=abc", []Option{WithLogQuery(), redact}, "Code=abc"},
		{"semicolon_separator", "/cb?x=1;access_token=secret;y=2", []Option{WithLogQuery(), redact}, "x=1;access_token=REDACTED;y=2"},
		{"trailing_separator", "/cb?code=abc&", []Option{WithLogQuery(), redact}, "code=REDACTED&"},
		{"bad_escape_kept", "/cb?%zz=1&code=x", []Option{WithLogQuery(), redact}, "%zz=1&code=REDACTED"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, queryField(t, tc.target, tc.opts...))
		})
	}
}

func TestSeeOtherLoggedByDefault(t *testing.T) {
	t.Parallel()

	for _, code := range []int{http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		require.True(t, shouldLogStatusCode(code, DefaultLogStatusCodesSet, nil), code)
	}
}

func TestSlogWithLevels(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	l := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	lh := SlogWithLevels(l, map[int]slog.Level{http.StatusUnauthorized: slog.LevelInfo, http.StatusNotFound: slog.LevelDebug})

	level := func(code int) string {
		buf.Reset()
		lh.Log(t.Context(), "request completed", code, nil)
		var entry struct{ Level string }
		require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
		return entry.Level
	}
	require.Equal(t, "INFO", level(http.StatusUnauthorized), "override")
	require.Equal(t, "DEBUG", level(http.StatusNotFound), "override")
	require.Equal(t, "ERROR", level(http.StatusForbidden), "default mapping")
	require.Equal(t, "WARN", level(http.StatusConflict), "default mapping")
	require.Equal(t, "ERROR", level(http.StatusInternalServerError), "default mapping")
}
