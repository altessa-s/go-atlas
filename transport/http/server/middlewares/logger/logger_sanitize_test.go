// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package logger

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/transport/observability"

	slogx "github.com/altessa-s/go-slogx"
)

// loggedFields runs one request through the middleware and returns the fields
// of its completion record.
func loggedFields(t *testing.T, req *http.Request, opts ...Option) map[string]any {
	t.Helper()
	var got map[string]any
	lh := LogHandlerFunc(func(_ context.Context, _ string, _ int, fields slogx.Fields) { got = fieldMap(fields) })
	h := New(lh, opts...).Handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	h.ServeHTTP(httptest.NewRecorder(), req)
	require.NotNil(t, got, "the request must be logged")
	return got
}

// A decoded path can carry CR/LF and other control characters; logged raw they
// forge log lines.
func TestHandler_StripsControlCharacters(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/items/%0d%0alevel=ERROR%20msg=forged%1b%5b31m%00%7f%c2%85", nil)
	req.Header.Set("User-Agent", "agent\twith\ttabs")
	got := loggedFields(t, req)

	require.Equal(t, "/items/level=ERROR msg=forged[31m", got[FieldKeyHTTPPath])
	require.Equal(t, "agentwithtabs", got[observability.FieldKeyClientUserAgent])
}

func TestHandler_CapsPathAndUserAgent(t *testing.T) {
	t.Parallel()

	t.Run("defaults", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/"+strings.Repeat("a", 3*DefaultMaxPathLength), nil)
		req.Header.Set("User-Agent", strings.Repeat("u", 3*DefaultMaxUserAgentLength))
		got := loggedFields(t, req)

		path, ok := got[FieldKeyHTTPPath].(string)
		require.True(t, ok)
		require.Len(t, path, DefaultMaxPathLength)
		require.True(t, strings.HasSuffix(path, TruncationSuffix))

		ua, ok := got[observability.FieldKeyClientUserAgent].(string)
		require.True(t, ok)
		require.Len(t, ua, DefaultMaxUserAgentLength)
		require.True(t, strings.HasSuffix(ua, TruncationSuffix))
	})

	t.Run("configured caps", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/abcdefghijklmnop", nil)
		req.Header.Set("User-Agent", "0123456789abcdef")
		got := loggedFields(t, req, WithMaxPathLength(8), WithMaxUserAgentLength(6))

		require.Equal(t, "/abcd...", got[FieldKeyHTTPPath])
		require.Equal(t, "012...", got[observability.FieldKeyClientUserAgent])
	})

	t.Run("values within the caps are logged as is", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/items/42", nil)
		req.Header.Set("User-Agent", "curl/8.4.0")
		got := loggedFields(t, req)

		require.Equal(t, "/api/v1/items/42", got[FieldKeyHTTPPath])
		require.Equal(t, "curl/8.4.0", got[observability.FieldKeyClientUserAgent])
	})

	t.Run("a non-positive cap keeps the default", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/items/42", nil)
		got := loggedFields(t, req, WithMaxPathLength(0), WithMaxUserAgentLength(-1))

		require.Equal(t, "/api/v1/items/42", got[FieldKeyHTTPPath])
	})
}

func TestSanitizeLogValue(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		in     string
		maxLen int
		want   string
	}{
		{"clean", "/a/b", 100, "/a/b"},
		{"empty", "", 100, ""},
		{"CR and LF", "a\r\nb", 100, "ab"},
		{"NUL, ESC and DEL", "a\x00b\x1bc\x7fd", 100, "abcd"},
		{"C1 control", "a\u0085b", 100, "ab"},
		{"non-ASCII text is kept", "/café/日本", 100, "/café/日本"},
		{"cut on a rune boundary", "ééééé", 6, "é..."},
		{"control characters do not count after the cut", "ab\ncd\nef", 6, "ab..."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, sanitizeLogValue(tc.in, tc.maxLen))
		})
	}
}
