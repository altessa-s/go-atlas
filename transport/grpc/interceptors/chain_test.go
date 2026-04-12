// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package interceptors

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewChain_Empty(t *testing.T) {
	c := NewChain()
	require.Equal(t, 0, c.Len())
}

func TestNewChain_WithInterceptors(t *testing.T) {
	c := NewChain(&NoOpInterceptor{}, &NoOpClientInterceptor{})
	require.Equal(t, 2, c.Len())
}

func TestNewChain_IgnoresInvalidTypes(t *testing.T) {
	c := NewChain("string", 42, &NoOpInterceptor{})
	require.Equal(t, 1, c.Len())
}

func TestChain_ServerOptions(t *testing.T) {
	c := NewChain(&NoOpInterceptor{})
	opts, err := c.ServerOptions()
	require.NoError(t, err)
	require.NotEqual(t, 0, len(opts))
}

func TestChain_ClientOptions(t *testing.T) {
	c := NewChain(&NoOpClientInterceptor{})
	opts, err := c.ClientOptions()
	require.NoError(t, err)
	require.NotEqual(t, 0, len(opts))
}

func TestChain_DependencyOrder(t *testing.T) {
	c := NewChain(&NoOpInterceptor{})
	names, err := c.DependencyOrder()
	require.NoError(t, err)
	require.NotEqual(t, 0, len(names))
	// metadata should be auto-added first
	require.Equal(t, "metadata", names[0])
}

func TestChain_DependencyGraph(t *testing.T) {
	c := NewChain(&NoOpInterceptor{})
	graph := c.DependencyGraph()
	_, ok := graph["noop"]
	require.True(t, ok, "expected noop in graph")
}

func TestChain_WithLogger(t *testing.T) {
	c := NewChain()
	ret := c.WithLogger(slog.New(slog.DiscardHandler))
	require.Equal(t, c, ret)
}
