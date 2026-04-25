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

func TestNewChain(t *testing.T) {
	c := NewChain()
	require.Equal(t, 0, c.Len())
}

func TestChain_Add(t *testing.T) {
	c := NewChain()
	c.Add(Noop("a"), Noop("b"))
	require.Equal(t, 2, c.Len())
}

func TestChain_AddFunc(t *testing.T) {
	c := NewChain()
	c.AddFunc("test", func(next http.Handler) http.Handler { return next })
	require.Equal(t, 1, c.Len())
}

func TestChain_Has(t *testing.T) {
	c := NewChain(Noop("a"))
	require.True(t, c.Has("a"), "should have a")
	require.False(t, c.Has("b"), "should not have b")
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
	require.Equal(t, "ABH", order)
}

func TestChain_ThenFunc(t *testing.T) {
	c := NewChain()
	called := false
	handler := c.ThenFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	require.True(t, called, "handler not called")
}

func TestChain_Clone(t *testing.T) {
	c := NewChain(Noop("a"))
	clone := c.Clone()
	clone.Add(Noop("b"))
	require.Equal(t, 1, c.Len())
}

func TestChain_Extend(t *testing.T) {
	c := NewChain(Noop("a"))
	ext := c.Extend(Noop("b"))
	require.Equal(t, 2, ext.Len())
	require.Equal(t, 1, c.Len())
}

func TestChain_Middlewares(t *testing.T) {
	c := NewChain(Noop("a"), Noop("b"))
	mws := c.Middlewares()
	require.Len(t, mws, 2)
}

func TestChain_DependencyOrder(t *testing.T) {
	c := NewChain(Noop("a"), Noop("b"))
	names, err := c.DependencyOrder()
	require.NoError(t, err)
	require.Len(t, names, 2)
}

func TestChain_DependencyGraph(t *testing.T) {
	c := NewChain(Noop("a"))
	graph := c.DependencyGraph()
	_, ok := graph["a"]
	require.True(t, ok, "expected a in graph")
}

func TestChain_NilMiddleware(t *testing.T) {
	c := NewChain(nil, Noop("a"), nil)
	require.Equal(t, 1, c.Len())
}

func TestChain_Then_NilHandler(t *testing.T) {
	c := NewChain()
	handler := c.Then(nil)
	require.NotNil(t, handler)
}
