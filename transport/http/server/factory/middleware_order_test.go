// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/recovery"
)

type namedMiddleware struct {
	middlewares.BaseMiddleware
}

func (m *namedMiddleware) Handler(next http.Handler) http.Handler { return next }
func (m *namedMiddleware) Dependencies() []string                 { return nil }

func newNamed(name string) middlewares.Middleware {
	return &namedMiddleware{BaseMiddleware: middlewares.NewBaseMiddleware(name, nil)}
}

func newRecoveryFake() middlewares.Middleware {
	return &namedMiddleware{BaseMiddleware: middlewares.NewBaseMiddleware(recovery.Name(), nil)}
}

// TestPinRecoveryOutermost_MovesRecoveryToHead is the regression guard
// for the panic-coverage fix. Even when topological order placed
// recovery deep in the chain (because requestid is its declared
// dependency), the post-pass must hoist recovery to index 0 so it
// wraps every other middleware — including the one it depends on.
func TestPinRecoveryOutermost_MovesRecoveryToHead(t *testing.T) {
	in := []middlewares.Middleware{
		newNamed("requestid"),
		newNamed("logger"),
		newRecoveryFake(),
		newNamed("metrics"),
	}

	out := pinRecoveryOutermost(in)

	require.Len(t, out, 4)
	require.Equal(t, recovery.Name(), out[0].Name(),
		"recovery must be hoisted to position 0 so it wraps every other middleware")
	require.Equal(t, "requestid", out[1].Name(),
		"non-recovery order must be preserved relative to itself")
	require.Equal(t, "logger", out[2].Name())
	require.Equal(t, "metrics", out[3].Name())
}

// TestPinRecoveryOutermost_NoOpWhenAlreadyFirst confirms the helper
// short-circuits when recovery is already at position 0 (the normal
// case after a future fix to the dependency declaration).
func TestPinRecoveryOutermost_NoOpWhenAlreadyFirst(t *testing.T) {
	in := []middlewares.Middleware{
		newRecoveryFake(),
		newNamed("requestid"),
		newNamed("logger"),
	}

	out := pinRecoveryOutermost(in)

	require.Equal(t, recovery.Name(), out[0].Name())
	require.Equal(t, "requestid", out[1].Name())
	require.Equal(t, "logger", out[2].Name())
}

// TestPinRecoveryOutermost_NoRecoveryIsHarmless pins the safety
// guarantee that a chain WITHOUT recovery passes through untouched —
// no panic, no reordering.
func TestPinRecoveryOutermost_NoRecoveryIsHarmless(t *testing.T) {
	in := []middlewares.Middleware{
		newNamed("requestid"),
		newNamed("logger"),
	}

	out := pinRecoveryOutermost(in)

	require.Equal(t, "requestid", out[0].Name())
	require.Equal(t, "logger", out[1].Name())
}
