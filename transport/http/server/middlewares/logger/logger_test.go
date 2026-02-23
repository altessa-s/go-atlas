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
	if !called {
		t.Fatal("nil handler should pass through")
	}
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

	if !logged {
		t.Fatal("should log request")
	}
	if loggedStatus != http.StatusOK {
		t.Fatalf("status = %d", loggedStatus)
	}
}

func TestMiddleware_IgnoresPath(t *testing.T) {
	logged := false
	lh := LogHandlerFunc(func(ctx context.Context, msg string, statusCode int, fields slogx.Fields) {
		logged = true
	})

	mw := Middleware(lh, WithIgnorePaths("/health"))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/health", nil))
	if logged {
		t.Fatal("should not log ignored path")
	}
}

func TestMiddleware_IgnoresMethod(t *testing.T) {
	logged := false
	lh := LogHandlerFunc(func(ctx context.Context, msg string, statusCode int, fields slogx.Fields) {
		logged = true
	})

	mw := Middleware(lh, WithIgnoreMethods("OPTIONS"))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("OPTIONS", "/test", nil))
	if logged {
		t.Fatal("should not log ignored method")
	}
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
	if logged {
		t.Fatal("should not log ignored response code")
	}
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
		if got := httpStatusToLevel(tt.code); got != tt.want {
			t.Fatalf("httpStatusToLevel(%d) = %v, want %v", tt.code, got, tt.want)
		}
	}
}

func TestShouldLogStatusCode(t *testing.T) {
	logSet := map[int]struct{}{200: {}, 404: {}}
	ignoreSet := map[int]struct{}{200: {}}

	if shouldLogStatusCode(200, logSet, ignoreSet) {
		t.Fatal("200 should be ignored")
	}
	if !shouldLogStatusCode(404, logSet, ignoreSet) {
		t.Fatal("404 should be logged")
	}
	if shouldLogStatusCode(500, logSet, ignoreSet) {
		t.Fatal("500 not in log set")
	}
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

	if strings.Contains(reqContent, "secret") {
		t.Fatalf("request body not redacted: %s", reqContent)
	}
	if !strings.Contains(reqContent, "[REDACTED]") {
		t.Fatalf("request body missing redaction marker: %s", reqContent)
	}
	if strings.Contains(respContent, "secret") {
		t.Fatalf("response body not redacted: %s", respContent)
	}
	if !strings.Contains(respContent, "[REDACTED]") {
		t.Fatalf("response body missing redaction marker: %s", respContent)
	}
}

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

	if reqContent != "request-body" {
		t.Fatalf("request body = %q, want %q", reqContent, "request-body")
	}
	if respContent != "response-body" {
		t.Fatalf("response body = %q, want %q", respContent, "response-body")
	}
}
