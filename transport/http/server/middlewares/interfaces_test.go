// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMatchFunc(t *testing.T) {
	if !(MatchFunc(func() bool { return true })).Match() {
		t.Fatal("expected true")
	}
	if (MatchFunc(func() bool { return false })).Match() {
		t.Fatal("expected false")
	}
}

func TestFunc(t *testing.T) {
	called := false
	m := Func("test", func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			next.ServeHTTP(w, r)
		})
	})
	if m.Name() != "test" {
		t.Fatalf("Name() = %q", m.Name())
	}

	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if !called {
		t.Fatal("middleware not called")
	}
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
		if rec.Code != http.StatusTeapot {
			t.Fatalf("code = %d", rec.Code)
		}
	})

	t.Run("no_match", func(t *testing.T) {
		m := ConditionalMiddleware(MatchFunc(func() bool { return false }), inner)
		rec := httptest.NewRecorder()
		m.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("code = %d", rec.Code)
		}
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
		if m != inner {
			t.Fatal("expected same middleware")
		}
	})

	t.Run("false", func(t *testing.T) {
		m := StaticConditionMiddleware(false, inner)
		if m.Name() != "inner" {
			t.Fatalf("Name() = %q", m.Name())
		}
		rec := httptest.NewRecorder()
		m.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("code = %d, noop should pass through", rec.Code)
		}
	})
}

func TestNoop(t *testing.T) {
	m := Noop("test")
	if m.Name() != "test" {
		t.Fatalf("Name() = %q", m.Name())
	}
	called := false
	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if !called {
		t.Fatal("next not called")
	}
}
