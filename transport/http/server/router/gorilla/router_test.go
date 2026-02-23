// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package gorilla

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/altessa-s/go-atlas/transport/http/server/router"
)

func TestNew(t *testing.T) {
	r := New()
	if r == nil {
		t.Fatal("New() returned nil")
	}
}

func TestRouter_ImplementsInterface(t *testing.T) {
	var _ router.Router = New()
}

func TestRouter_Handle(t *testing.T) {
	r := New()
	called := false
	r.Handle("/test", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	r.ServeHTTP(rec, req)

	if !called {
		t.Fatal("handler was not called")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestRouter_HandleFunc(t *testing.T) {
	r := New()
	called := false
	r.HandleFunc("/test", func(w http.ResponseWriter, req *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	r.ServeHTTP(rec, req)

	if !called {
		t.Fatal("handler was not called")
	}
}

func TestRouter_Methods(t *testing.T) {
	r := New()
	r.HandleFunc("/method", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	}).Methods("POST")

	t.Run("allowed", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/method", nil)
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
	})

	t.Run("not_allowed", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/method", nil)
		r.ServeHTTP(rec, req)
		if rec.Code == http.StatusOK {
			t.Fatal("GET should not match POST-only route")
		}
	})
}

func TestRouter_PathPrefix(t *testing.T) {
	r := New()
	sub := r.PathPrefix("/api").Subrouter()
	sub.HandleFunc("/users", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/users", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestRouter_Use_Middleware(t *testing.T) {
	r := New()
	middlewareCalled := false
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			middlewareCalled = true
			next.ServeHTTP(w, req)
		})
	})
	r.HandleFunc("/test", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	r.ServeHTTP(rec, req)

	if !middlewareCalled {
		t.Fatal("middleware was not called")
	}
}

func TestRouter_Subrouter(t *testing.T) {
	r := New()
	sub := r.Subrouter()
	if sub == nil {
		t.Fatal("Subrouter() returned nil")
	}
}

func TestRouter_Underlying(t *testing.T) {
	r := New()
	if r.Underlying() == nil {
		t.Fatal("Underlying() returned nil")
	}
}

func TestRoute_Handler(t *testing.T) {
	r := New()
	called := false
	r.HandleFunc("/path", nil).Handler(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/path", nil)
	r.ServeHTTP(rec, req)

	if !called {
		t.Fatal("handler was not called")
	}
}

func TestRoute_Path(t *testing.T) {
	r := New()
	r.Methods("GET").Path("/specific").HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/specific", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestRoute_Subrouter(t *testing.T) {
	r := New()
	route := r.PathPrefix("/api")
	sub := route.Subrouter()
	if sub == nil {
		t.Fatal("route.Subrouter() returned nil")
	}
}
