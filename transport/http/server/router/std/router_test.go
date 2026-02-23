// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package std

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
	r.HandleFunc("/func", func(w http.ResponseWriter, req *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/func", nil)
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
		// std mux won't match "POST /method" for GET request
		if rec.Code == http.StatusOK {
			t.Fatal("GET should not match POST-only route")
		}
	})
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
	r.HandleFunc("/mw", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/mw", nil)
	r.ServeHTTP(rec, req)

	if !middlewareCalled {
		t.Fatal("middleware was not called")
	}
}

func TestRouter_PathPrefix(t *testing.T) {
	r := New()
	r.PathPrefix("/api").HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/anything", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestRouter_Subrouter(t *testing.T) {
	r := New()
	sub := r.Subrouter()
	if sub == nil {
		t.Fatal("Subrouter() returned nil")
	}
}

func TestRouter_Initialize(t *testing.T) {
	r := New()
	r.HandleFunc("/init", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Initialize()

	// After initialize, adding routes should panic
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic after Initialize")
		}
	}()
	r.HandleFunc("/after-init", func(w http.ResponseWriter, req *http.Request) {})
}

func TestRouter_Initialize_ThenServe(t *testing.T) {
	r := New()
	r.HandleFunc("/serve", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Initialize()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/serve", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestRoute_Methods_Chain(t *testing.T) {
	r := New()
	r.HandleFunc("/chain", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	}).Methods("GET", "POST")

	for _, method := range []string{"GET", "POST"} {
		t.Run(method, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(method, "/chain", nil)
			r.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d for %s", rec.Code, method)
			}
		})
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

func TestRoute_MissingHandler_Panics(t *testing.T) {
	r := New()
	r.HandleFunc("/ok", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Add route with no handler - should panic during initialization
	rt := &route{router: r, pattern: "/no-handler"}
	r.routes = append(r.routes, rt)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for missing handler")
		}
	}()
	r.Initialize()
}

func TestRoute_MissingPath_Panics(t *testing.T) {
	r := New()
	rt := &route{router: r, handler: http.NotFoundHandler()}
	r.routes = append(r.routes, rt)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for missing path")
		}
	}()
	r.Initialize()
}

func TestRouter_MultipleMiddleware(t *testing.T) {
	r := New()
	order := ""
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			order += "A"
			next.ServeHTTP(w, req)
		})
	})
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			order += "B"
			next.ServeHTTP(w, req)
		})
	})
	r.HandleFunc("/order", func(w http.ResponseWriter, req *http.Request) {
		order += "H"
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/order", nil)
	r.ServeHTTP(rec, req)

	if order != "ABH" {
		t.Fatalf("middleware order = %q, want ABH", order)
	}
}
