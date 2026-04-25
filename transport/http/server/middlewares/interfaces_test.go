// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMatchFunc(t *testing.T) {
	require.True(t, (MatchFunc(func() bool { return true })).Match(), "expected true")
	require.False(t, (MatchFunc(func() bool { return false })).Match(), "expected false")
}

func TestFunc(t *testing.T) {
	called := false
	m := Func("test", func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			next.ServeHTTP(w, r)
		})
	})
	require.Equal(t, "test", m.Name())

	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	require.True(t, called, "middleware not called")
}

func TestConditionalMiddleware(t *testing.T) {
	inner := Func("inner", func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		})
	})

	t.Run("match", func(t *testing.T) {
		m := ConditionalMiddleware(MatchFunc(func() bool { return true }), inner)
		rec := httptest.NewRecorder()
		m.Handler(http.NotFoundHandler()).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
		require.Equal(t, http.StatusTeapot, rec.Code)
	})

	t.Run("no_match", func(t *testing.T) {
		m := ConditionalMiddleware(MatchFunc(func() bool { return false }), inner)
		rec := httptest.NewRecorder()
		m.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
		require.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestStaticConditionMiddleware(t *testing.T) {
	inner := Func("inner", func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		})
	})

	t.Run("true", func(t *testing.T) {
		m := StaticConditionMiddleware(true, inner)
		require.Equal(t, inner, m)
	})

	t.Run("false", func(t *testing.T) {
		m := StaticConditionMiddleware(false, inner)
		require.Equal(t, "inner", m.Name())
		rec := httptest.NewRecorder()
		m.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
		require.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestNoop(t *testing.T) {
	m := Noop("test")
	require.Equal(t, "test", m.Name())
	called := false
	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	require.True(t, called, "next not called")
}
