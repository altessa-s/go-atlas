// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewChain(t *testing.T) {
	c := NewChain()
	if c.Len() != 0 {
		t.Fatalf("Len() = %d", c.Len())
	}
}

func TestChain_Add(t *testing.T) {
	c := NewChain()
	c.Add(Noop("a"), Noop("b"))
	if c.Len() != 2 {
		t.Fatalf("Len() = %d", c.Len())
	}
}

func TestChain_AddFunc(t *testing.T) {
	c := NewChain()
	c.AddFunc("test", func(next http.Handler) http.Handler { return next })
	if c.Len() != 1 {
		t.Fatalf("Len() = %d", c.Len())
	}
}

func TestChain_Has(t *testing.T) {
	c := NewChain(Noop("a"))
	if !c.Has("a") {
		t.Fatal("should have a")
	}
	if c.Has("b") {
		t.Fatal("should not have b")
	}
}

func TestChain_Then(t *testing.T) {
	order := ""
	a := Func("a", func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order += "A"
			next.ServeHTTP(w, r)
		})
	})
	b := Func("b", func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order += "B"
			next.ServeHTTP(w, r)
		})
	})

	c := NewChain(a, b)
	handler := c.Then(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		order += "H"
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if order != "ABH" {
		t.Fatalf("order = %q, want ABH", order)
	}
}

func TestChain_ThenFunc(t *testing.T) {
	c := NewChain()
	called := false
	handler := c.ThenFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if !called {
		t.Fatal("handler not called")
	}
}

func TestChain_Clone(t *testing.T) {
	c := NewChain(Noop("a"))
	clone := c.Clone()
	clone.Add(Noop("b"))
	if c.Len() != 1 {
		t.Fatal("clone should not affect original")
	}
}

func TestChain_Extend(t *testing.T) {
	c := NewChain(Noop("a"))
	ext := c.Extend(Noop("b"))
	if ext.Len() != 2 {
		t.Fatalf("Len() = %d", ext.Len())
	}
	if c.Len() != 1 {
		t.Fatal("original should not change")
	}
}

func TestChain_Middlewares(t *testing.T) {
	c := NewChain(Noop("a"), Noop("b"))
	mws := c.Middlewares()
	if len(mws) != 2 {
		t.Fatalf("len = %d", len(mws))
	}
}

func TestChain_DependencyOrder(t *testing.T) {
	c := NewChain(Noop("a"), Noop("b"))
	names, err := c.DependencyOrder()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 {
		t.Fatalf("len = %d", len(names))
	}
}

func TestChain_DependencyGraph(t *testing.T) {
	c := NewChain(Noop("a"))
	graph := c.DependencyGraph()
	if _, ok := graph["a"]; !ok {
		t.Fatal("expected a in graph")
	}
}

func TestChain_NilMiddleware(t *testing.T) {
	c := NewChain(nil, Noop("a"), nil)
	if c.Len() != 1 {
		t.Fatalf("Len() = %d, nils should be ignored", c.Len())
	}
}

func TestChain_Then_NilHandler(t *testing.T) {
	c := NewChain()
	handler := c.Then(nil)
	if handler == nil {
		t.Fatal("should return DefaultServeMux")
	}
}
