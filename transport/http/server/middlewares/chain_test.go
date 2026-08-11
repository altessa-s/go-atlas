// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/internal/depgraph"
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

// TestOrdering_DuplicateNamesAreRejectedOrDeduped pins the distinction the two
// ordering entry points are named for.
//
// They used to be identical: ordering is keyed by name, so the graph collapsed
// a repeat on its own and the "WithDedupe" pass that ran afterwards could never
// remove anything. A caller who deliberately chose the plain variant to keep
// both middlewares got one anyway, with nothing to say so.
//
// Now the plain variant refuses the repeat and the dedupe variant discards it
// on purpose — which is the only way a discard can be the caller's decision
// rather than the graph's silence.
func TestOrdering_DuplicateNamesAreRejectedOrDeduped(t *testing.T) {
	t.Parallel()

	first := Func("limiter", func(next http.Handler) http.Handler { return next })
	second := Func("limiter", func(next http.Handler) http.Handler { return next })
	logger := Func("logger", func(next http.Handler) http.Handler { return next })

	list := []Middleware{first, logger, second}

	t.Run("plain ordering refuses", func(t *testing.T) {
		t.Parallel()

		ordered, err := OrderMiddlewares(list)
		require.ErrorIs(t, err, depgraph.ErrDuplicateName)
		require.Nil(t, ordered, "a rejected list must not also produce an ordering")
	})

	t.Run("dedupe variant keeps the first", func(t *testing.T) {
		t.Parallel()

		ordered, err := OrderMiddlewaresWithDedupe(list)
		require.NoError(t, err)
		require.Len(t, ordered, 2)

		names := make([]string, 0, len(ordered))
		for _, m := range ordered {
			names = append(names, m.Name())
		}
		require.ElementsMatch(t, []string{"limiter", "logger"}, names)
	})

	t.Run("chain methods agree with the functions", func(t *testing.T) {
		t.Parallel()

		_, err := NewChain(list...).Ordered()
		require.ErrorIs(t, err, depgraph.ErrDuplicateName)

		deduped, err := NewChain(list...).OrderedWithDedupe()
		require.NoError(t, err)
		require.Equal(t, 2, deduped.Len())
	})
}
