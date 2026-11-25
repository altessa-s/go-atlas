// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package middlewares_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/transport/depgraph"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"
)

type outerGuard struct {
	middlewares.Middleware
	after []string
}

func (outerGuard) Outermost() bool          { return true }
func (g outerGuard) Dependencies() []string { return g.after }

func TestOutermostDependencyOrder(t *testing.T) {
	t.Parallel()
	pass := func(h http.Handler) http.Handler { return h }
	first := outerGuard{middlewares.Noop("first"), nil}
	second := outerGuard{middlewares.Noop("second"), []string{"first"}}
	inner := middlewares.FuncWithDeps("inner", []string{"first"}, pass)
	ordered, err := middlewares.OrderMiddlewares([]middlewares.Middleware{inner, second, first})
	require.NoError(t, err)
	require.Equal(t, []string{"first", "second", "inner"}, names(ordered))

	conditional := middlewares.ConditionalMiddleware(middlewares.MatchFunc(func() bool { return true }), second)
	ordered, err = middlewares.OrderMiddlewares([]middlewares.Middleware{inner, conditional, first})
	require.NoError(t, err)
	require.Equal(t, []string{"first", "second", "inner"}, names(ordered))

	first.after = []string{"inner"}
	_, err = middlewares.OrderMiddlewares([]middlewares.Middleware{first, middlewares.Noop("inner")})
	require.ErrorIs(t, err, depgraph.ErrCyclicDependency)
}

type requiredOuterGuard struct{ outerGuard }

func (g requiredOuterGuard) RequiredDependencies() []string { return g.after }

func TestConditionalOutermostRequiresPeer(t *testing.T) {
	t.Parallel()
	guard := requiredOuterGuard{outerGuard{middlewares.Noop("guard"), []string{"peer"}}}
	conditional := middlewares.ConditionalMiddleware(middlewares.MatchFunc(func() bool { return true }), guard)
	_, err := middlewares.OrderMiddlewares([]middlewares.Middleware{conditional})
	require.ErrorIs(t, err, depgraph.ErrMissingDependency)
}
