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

type recNode struct {
	Name string   `yaml:"name" default:"node"`
	Next *recNode `yaml:"next"`
}

type recConfig struct {
	Head recNode  `yaml:"head"`
	Root *recNode `yaml:"root"`
}

// TestLoad_RecursiveTypeNil checks that a configuration with a recursive
// pointer type loads while the recursive sections are nil.
func TestLoad_RecursiveTypeNil(t *testing.T) {
	t.Parallel()

	cfg := &recConfig{}
	_, err := loader.New(nil, loader.WithSkipEnv()).Load(cfg)
	require.NoError(t, err)
	require.Equal(t, "node", cfg.Head.Name)
	require.Nil(t, cfg.Head.Next)
}

// TestLoad_RecursiveTypeChain checks that a populated chain from a file is
// decoded and defaulted node by node.
func TestLoad_RecursiveTypeChain(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("head:\n  next:\n    name: second\n    next:\n      next: {}\n"), 0o600))

	cfg := &recConfig{}
	_, err := loader.New(nil, loader.WithPath(path), loader.WithSkipEnv()).Load(cfg)
	require.NoError(t, err)
	require.Equal(t, "node", cfg.Head.Name)
	require.Equal(t, "second", cfg.Head.Next.Name)
	require.Equal(t, "node", cfg.Head.Next.Next.Name)
	require.Equal(t, "node", cfg.Head.Next.Next.Next.Name)
	require.Nil(t, cfg.Head.Next.Next.Next.Next)
}

// TestLoad_RecursiveTypeEnv checks that environment variables reach nodes of
// a recursive type below its first level, allocating the nodes on the way.
// It mutates the process environment via t.Setenv, so it is not parallel.
func TestLoad_RecursiveTypeEnv(t *testing.T) {
	t.Setenv("ENVREC_HEAD__NAME", "first")
	t.Setenv("ENVREC_HEAD__NEXT__NEXT__NAME", "third")
	t.Setenv("ENVREC_ROOT__NEXT__NAME", "root-second")

	cfg := &recConfig{}
	_, err := loader.New(nil, loader.WithEnvPrefix("ENVREC")).Load(cfg)
	require.NoError(t, err)
	require.Equal(t, "first", cfg.Head.Name)
	require.Equal(t, "node", cfg.Head.Next.Name)
	require.Equal(t, "third", cfg.Head.Next.Next.Name)
	require.Nil(t, cfg.Head.Next.Next.Next)
	require.NotNil(t, cfg.Root)
	require.Equal(t, "root-second", cfg.Root.Next.Name)
}
