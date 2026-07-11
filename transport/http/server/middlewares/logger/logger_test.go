// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package logger

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/internal/observability"

	slogx "github.com/altessa-s/go-atlas/observability/slog"
)

func TestMiddleware_NilHandler(t *testing.T) {
	mw := Middleware(nil)
	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/test", nil))
	require.True(t, called, "nil handler should pass through")
}

func TestMiddleware_LogsRequest(t *testing.T) {
	var logged bool
	var loggedStatus int
	lh := LogHandlerFunc(func(ctx context.Context, msg string, statusCode int, fields slogx.Fields) {
		logged = true
		loggedStatus = statusCode
	})

	mw := Middleware(lh)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/test", nil))

	require.True(t, logged, "should log request")
	require.Equal(t, http.StatusOK, loggedStatus)
}

func TestMiddleware_IgnoresPath(t *testing.T) {
	logged := false
	lh := LogHandlerFunc(func(ctx context.Context, msg string, statusCode int, fields slogx.Fields) {
		logged = true
	})

	mw := Middleware(lh, WithIgnorePaths("/health"))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/health", nil))
	require.False(t, logged, "should not log ignored path")
}

func TestMiddleware_IgnoresMethod(t *testing.T) {
	logged := false
	lh := LogHandlerFunc(func(ctx context.Context, msg string, statusCode int, fields slogx.Fields) {
		logged = true
	})

	mw := Middleware(lh, WithIgnoreMethods("OPTIONS"))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("OPTIONS", "/test", nil))
	require.False(t, logged, "should not log ignored method")
}

func TestMiddleware_IgnoresResponseCode(t *testing.T) {
	logged := false
	lh := LogHandlerFunc(func(ctx context.Context, msg string, statusCode int, fields slogx.Fields) {
		logged = true
	})

	mw := Middleware(lh, WithIgnoreResponseCodes(200))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/test", nil))
	require.False(t, logged, "should not log ignored response code")
}

func TestSlog(t *testing.T) {
	lh := Slog(slog.New(slog.DiscardHandler))
	lh.Log(t.Context(), "test", http.StatusOK, nil)
}

func TestHttpStatusToLevel(t *testing.T) {
	tests := []struct {
		code int
		want slog.Level
	}{
		{200, slog.LevelInfo},
		{400, slog.LevelWarn},
		{401, slog.LevelError},
		{403, slog.LevelError},
		{404, slog.LevelWarn},
		{500, slog.LevelError},
	}
	for _, tt := range tests {
		got := httpStatusToLevel(tt.code)
		require.Equal(t, tt.want, got)
	}
}

func TestShouldLogStatusCode(t *testing.T) {
	logSet := map[int]struct{}{200: {}, 404: {}}
	ignoreSet := map[int]struct{}{200: {}}

	require.False(t, shouldLogStatusCode(200, logSet, ignoreSet), "200 should be ignored")
	require.True(t, shouldLogStatusCode(404, logSet, ignoreSet), "404 should be logged")
	require.False(t, shouldLogStatusCode(500, logSet, ignoreSet), "500 not in log set")
}

func TestMiddleware_BodyRedactor(t *testing.T) {
	var loggedFields slogx.Fields
	lh := LogHandlerFunc(func(ctx context.Context, msg string, statusCode int, fields slogx.Fields) {
		loggedFields = fields
	})

	redactor := func(body string) string {
		return strings.ReplaceAll(body, "secret", "[REDACTED]")
	}

	mw := Middleware(lh, WithLogRequest(), WithLogResponse(), WithBodyRedactor(redactor))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"token":"secret"}`))
	}))

	body := strings.NewReader(`{"password":"secret"}`)
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/test", body))

	var reqContent, respContent string
	for _, f := range loggedFields {
		if f.Key == observability.FieldKeyRequestContent {
			reqContent = f.Value.(string)
		}
		if f.Key == observability.FieldKeyResponseContent {
			respContent = f.Value.(string)
		}
	}

	require.False(t, strings.Contains(reqContent, "secret"), "request body not redacted: %s", reqContent)
	require.True(t, strings.Contains(reqContent, "[REDACTED]"))
	require.False(t, strings.Contains(respContent, "secret"), "response body not redacted: %s", respContent)
	require.True(t, strings.Contains(respContent, "[REDACTED]"))
}

// TestMiddleware_BodyRedactor_Nil pins the secure-by-default behavior: enabling
// body logging without a redactor must NOT write raw bodies. The middleware
// installs a fully-masking redactor, so bodies are replaced with the placeholder.
func TestMiddleware_BodyRedactor_Nil(t *testing.T) {
	var loggedFields slogx.Fields
	lh := LogHandlerFunc(func(ctx context.Context, msg string, statusCode int, fields slogx.Fields) {
		loggedFields = fields
	})

	mw := Middleware(lh, WithLogRequest(), WithLogResponse())
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`response-body`))
	}))

	body := strings.NewReader(`request-body`)
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/test", body))

	var reqContent, respContent string
	for _, f := range loggedFields {
		if f.Key == observability.FieldKeyRequestContent {
			reqContent = f.Value.(string)
		}
		if f.Key == observability.FieldKeyResponseContent {
			respContent = f.Value.(string)
		}
	}

	// Raw bodies must not leak; both are replaced by the safe placeholder.
	require.Equal(t, DefaultRedactedBodyPlaceholder, reqContent)
	require.Equal(t, DefaultRedactedBodyPlaceholder, respContent)
	require.NotContains(t, reqContent, "request-body")
	require.NotContains(t, respContent, "response-body")
}

// TestMiddleware_BodyRedactor_ExplicitPassthrough verifies the opt-in escape
// hatch: an identity redactor restores raw-body logging for callers that want it.
func TestMiddleware_BodyRedactor_ExplicitPassthrough(t *testing.T) {
	var loggedFields slogx.Fields
	lh := LogHandlerFunc(func(ctx context.Context, msg string, statusCode int, fields slogx.Fields) {
		loggedFields = fields
	})

	mw := Middleware(lh, WithLogRequest(), WithBodyRedactor(func(b string) string { return b }))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	body := strings.NewReader(`request-body`)
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/test", body))

	var reqContent string
	for _, f := range loggedFields {
		if f.Key == observability.FieldKeyRequestContent {
			reqContent = f.Value.(string)
		}
	}
	require.Equal(t, "request-body", reqContent)
}
