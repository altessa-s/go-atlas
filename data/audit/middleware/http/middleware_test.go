// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/audit"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	audithttp "github.com/altessa-s/go-atlas/data/audit/middleware/http"
)

func TestMiddleware_AuditsRequest(t *testing.T) {
	a, store := testhelpers.NewTestAuditor(t)

	handler := audithttp.Middleware(a)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/items", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	require.NoError(t, a.Shutdown(t.Context()))
	assert.Equal(t, 1, store.Len())

	events := store.Events()
	assert.Equal(t, audit.EventTypeAPIRequest, events[0].Type)
	assert.Equal(t, audit.ActionRead, events[0].Action)
	assert.Equal(t, "/api/v1/items", events[0].Resource.Path)
	assert.Equal(t, audit.ResultStatusSuccess, events[0].Result.Status)
}

func TestMiddleware_NilAuditor_PassesThrough(t *testing.T) {
	called := false
	handler := audithttp.Middleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestMiddleware_IgnorePaths(t *testing.T) {
	a, store := testhelpers.NewTestAuditor(t)

	handler := audithttp.Middleware(a,
		audithttp.WithIgnorePaths("/health", "/ready"),
	)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, path := range []string{"/health", "/ready"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	}

	// Non-ignored path
	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.NoError(t, a.Shutdown(t.Context()))
	assert.Equal(t, 1, store.Len())
}

func TestMiddleware_IgnoreMethods(t *testing.T) {
	a, store := testhelpers.NewTestAuditor(t)

	handler := audithttp.Middleware(a,
		audithttp.WithIgnoreMethods("OPTIONS", "HEAD"),
	)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/data", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	req = httptest.NewRequest(http.MethodPost, "/api/data", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.NoError(t, a.Shutdown(t.Context()))
	assert.Equal(t, 1, store.Len())
}

func TestMiddleware_HTTPMethodToAction(t *testing.T) {
	tests := []struct {
		method string
		action audit.Action
	}{
		{http.MethodPost, audit.ActionCreate},
		{http.MethodGet, audit.ActionRead},
		{http.MethodPut, audit.ActionUpdate},
		{http.MethodPatch, audit.ActionUpdate},
		{http.MethodDelete, audit.ActionDelete},
		{http.MethodHead, audit.ActionRead},
		{http.MethodOptions, audit.ActionRead},
		{"CUSTOM", audit.ActionExecute},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			a, store := testhelpers.NewTestAuditor(t)

			handler := audithttp.Middleware(a)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(tt.method, "/test", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			require.NoError(t, a.Shutdown(t.Context()))
			require.Equal(t, 1, store.Len())
			assert.Equal(t, tt.action, store.Events()[0].Action)
		})
	}
}

func TestMiddleware_StatusCodes(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   audit.ResultStatus
	}{
		{"success_200", http.StatusOK, audit.ResultStatusSuccess},
		{"success_201", http.StatusCreated, audit.ResultStatusSuccess},
		{"failure_400", http.StatusBadRequest, audit.ResultStatusFailure},
		{"failure_403", http.StatusForbidden, audit.ResultStatusFailure},
		{"error_500", http.StatusInternalServerError, audit.ResultStatusError},
		{"error_503", http.StatusServiceUnavailable, audit.ResultStatusError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, store := testhelpers.NewTestAuditor(t)

			handler := audithttp.Middleware(a)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			require.NoError(t, a.Shutdown(t.Context()))
			require.Equal(t, 1, store.Len())
			assert.Equal(t, tt.want, store.Events()[0].Result.Status)
			assert.Equal(t, tt.status, store.Events()[0].Result.Code)
		})
	}
}

func TestMiddleware_ActorExtractor(t *testing.T) {
	a, store := testhelpers.NewTestAuditor(t)

	handler := audithttp.Middleware(a,
		audithttp.WithActorExtractor(func(r *http.Request) audit.Actor {
			return audit.Actor{
				Type: audit.ActorTypeUser,
				ID:   r.Header.Get("X-User-ID"),
			}
		}),
	)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-User-ID", "user-42")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.NoError(t, a.Shutdown(t.Context()))
	require.Equal(t, 1, store.Len())
	assert.Equal(t, "user-42", store.Events()[0].Actor.ID)
}

func TestMiddleware_RequestIDExtractor(t *testing.T) {
	a, store := testhelpers.NewTestAuditor(t)

	type reqIDKey struct{}
	handler := audithttp.Middleware(a,
		audithttp.WithRequestIDExtractor(func(ctx context.Context) string {
			v, _ := ctx.Value(reqIDKey{}).(string)
			return v
		}),
	)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req = req.WithContext(context.WithValue(req.Context(), reqIDKey{}, "req-123"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.NoError(t, a.Shutdown(t.Context()))
	require.Equal(t, 1, store.Len())
	assert.Equal(t, "req-123", store.Events()[0].Context.RequestID)
}
