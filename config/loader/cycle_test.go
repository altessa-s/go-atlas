// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package loader_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config/loader"
)

// TestLoad_PointerCycle checks that a configuration whose pointers form a
// cycle loads: each node is defaulted once, and the cycle is kept.
func TestLoad_PointerCycle(t *testing.T) {
	t.Parallel()

	self := &recNode{}
	self.Next = self
	a, b := &recNode{}, &recNode{}
	a.Next, b.Next = b, a

	cfg := &recConfig{Root: self, Head: recNode{Next: a}}
	_, err := loader.New(nil, loader.WithSkipEnv()).Load(cfg)
	require.NoError(t, err)
	require.Same(t, self, cfg.Root.Next)
	require.Equal(t, "node", self.Name)
	require.Same(t, a, b.Next)
	require.Equal(t, "node", a.Name)
	require.Equal(t, "node", b.Name)
}

type cycDefaulted struct {
	Root *recNode `yaml:"root"`
}

func (c *cycDefaulted) Default() {}

// TestLoad_PointerCycleExplicit checks that a cycle under a value a file sets
// explicitly survives the copy kept while Default runs.
func TestLoad_PointerCycleExplicit(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("root: {}\n"), 0o600))

	self := &recNode{}
	self.Next = self
	cfg := &cycDefaulted{Root: self}
	_, err := loader.New(nil, loader.WithPath(path), loader.WithSkipEnv()).Load(cfg)
	require.NoError(t, err)
	require.NotNil(t, cfg.Root)
}

type EmbNode struct {
	*EmbNode

	Name string `yaml:"name"`
}

type embConfig struct {
	Root EmbNode `yaml:"root"`
}

// TestLoad_RecursiveEmbeddingUnmatched checks that a variable naming no field
// of a recursively embedded type is ignored without allocating embedded
// pointers, and that a matching one still sets its field.
// It mutates the process environment via t.Setenv, so it is not parallel.
func TestLoad_RecursiveEmbeddingUnmatched(t *testing.T) {
	t.Setenv("ENVEMB_ROOT__TYPO", "x")
	t.Setenv("ENVEMB_ROOT__NAME", "root")

	cfg := &embConfig{}
	_, err := loader.New(nil, loader.WithEnvPrefix("ENVEMB")).Load(cfg)
	require.NoError(t, err)
	require.Equal(t, "root", cfg.Root.Name)
	require.Nil(t, cfg.Root.EmbNode)
}

type CycA struct {
	*CycB
	*CycD
}

type CycB struct {
	*CycA
	*CycC
}

type CycC struct {
	Value string `yaml:"value"`
}

type CycD struct {
	Value string `yaml:"value"`
}

type cycEmbedConfig struct {
	Items []CycA `yaml:"items"`
}

// TestLoad_MutualRecursiveEmbedding checks that a field reached through
// mutually recursive embeddings, with matching fields on alternate branches,
// is set along one path, allocating only the embedded pointers on it (the
// defaults pass, which allocates embedded pointers on its own, is skipped).
// It mutates the process environment via t.Setenv, so it is not parallel.
func TestLoad_MutualRecursiveEmbedding(t *testing.T) {
	t.Setenv("ENVMUT_ITEMS__0__VALUE", "x")

	cfg := &cycEmbedConfig{}
	_, err := loader.New(nil, loader.WithEnvPrefix("ENVMUT"), loader.WithSkipDefaults()).Load(cfg)
	require.NoError(t, err)
	require.Len(t, cfg.Items, 1)
	require.NotNil(t, cfg.Items[0].CycB)
	require.NotNil(t, cfg.Items[0].CycC)
	require.Equal(t, "x", cfg.Items[0].CycC.Value)
	require.Nil(t, cfg.Items[0].CycB.CycA)
	require.Nil(t, cfg.Items[0].CycD)

	_, err = loader.New(nil, loader.WithEnvPrefix("ENVMUT")).Load(&cycEmbedConfig{})
	require.NoError(t, err)
}

type cycDefaultNode struct {
	Value string          `yaml:"value"`
	Next  *cycDefaultNode `yaml:"next"`
	calls int
}

func (n *cycDefaultNode) Default() {
	n.Value = "fallback"
	n.calls++
}

// TestLoad_RootCycleDefaultOnce checks that a pointer back to the root does
// not run Default on it again, so an explicit environment value survives.
// It mutates the process environment via t.Setenv, so it is not parallel.
func TestLoad_RootCycleDefaultOnce(t *testing.T) {
	t.Setenv("ENVROOTCYC_VALUE", "env")

	cfg := &cycDefaultNode{}
	cfg.Next = cfg
	_, err := loader.New(nil, loader.WithEnvPrefix("ENVROOTCYC")).Load(cfg)
	require.NoError(t, err)
	require.Equal(t, "env", cfg.Value)
	require.Equal(t, 1, cfg.calls)
	require.Same(t, cfg, cfg.Next)
}
