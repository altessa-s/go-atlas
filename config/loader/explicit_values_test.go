// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package loader_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config/loader"
	"github.com/altessa-s/go-atlas/config/loader/backend/toml"
)

type explicitNested struct {
	Enabled bool          `yaml:"enabled" default:"true"`
	Limit   int           `yaml:"limit" default:"5"`
	Timeout time.Duration `yaml:"timeout" default:"15m"`
}

type explicitItem struct {
	Name    string         `yaml:"name"`
	Enabled bool           `yaml:"enabled" default:"true"`
	Weight  int            `yaml:"weight" default:"10"`
	Inner   explicitNested `yaml:"inner"`
}

type explicitConfig struct {
	Enabled bool                    `yaml:"enabled" env:"EXPLICIT_ENABLED" default:"true"`
	Limit   int                     `yaml:"limit" env:"EXPLICIT_LIMIT" default:"5"`
	Ratio   float64                 `yaml:"ratio" default:"0.5"`
	Name    string                  `yaml:"name" default:"svc"`
	Timeout time.Duration           `yaml:"timeout" default:"15m"`
	Nested  explicitNested          `yaml:"nested"`
	Ptr     *explicitNested         `yaml:"ptr"`
	Items   []explicitItem          `yaml:"items"`
	PItems  []*explicitItem         `yaml:"pItems"`
	ByName  map[string]explicitItem `yaml:"byName"`
}

func loadExplicit(t *testing.T, content string) *explicitConfig {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	cfg := &explicitConfig{}
	_, err := loader.New(nil, loader.WithPath(path)).Load(cfg)
	require.NoError(t, err)
	return cfg
}

// An explicit false/0/"" in the file must win over a non-zero default tag.
func TestLoad_ExplicitZeroInFileWinsOverDefault(t *testing.T) {
	cfg := loadExplicit(t, `
enabled: false
limit: 0
ratio: 0
name: ""
timeout: "0s"
nested:
  enabled: false
  limit: 0
ptr:
  enabled: false
`)
	require.False(t, cfg.Enabled)
	require.Zero(t, cfg.Limit)
	require.Zero(t, cfg.Ratio)
	require.Empty(t, cfg.Name)
	require.Zero(t, cfg.Timeout)
	require.False(t, cfg.Nested.Enabled)
	require.Zero(t, cfg.Nested.Limit)
	require.Equal(t, 15*time.Minute, cfg.Nested.Timeout, "omitted nested field keeps its default")
	require.NotNil(t, cfg.Ptr)
	require.False(t, cfg.Ptr.Enabled)
	require.Equal(t, 5, cfg.Ptr.Limit, "omitted field of a file-created pointer struct gets its default")
}

// Omitted keys still receive their defaults, including inside collections.
func TestLoad_OmittedKeysGetDefaults(t *testing.T) {
	cfg := loadExplicit(t, `
items:
  - name: a
byName:
  b:
    name: b
`)
	require.True(t, cfg.Enabled)
	require.Equal(t, 5, cfg.Limit)
	require.Equal(t, "svc", cfg.Name)
	require.Equal(t, 15*time.Minute, cfg.Timeout)
	require.True(t, cfg.Nested.Enabled)
	require.Len(t, cfg.Items, 1)
	require.Equal(t, "a", cfg.Items[0].Name)
	require.Equal(t, 10, cfg.ByName["b"].Weight)
	require.True(t, cfg.ByName["b"].Enabled)
}

// An explicit false/0 from the environment must win over a non-zero default tag.
func TestLoad_ExplicitZeroInEnvWinsOverDefault(t *testing.T) {
	t.Setenv("EXPLICIT_ENABLED", "false")
	t.Setenv("EXPLICIT_LIMIT", "0")

	cfg := &explicitConfig{}
	_, err := loader.New(nil).Load(cfg)
	require.NoError(t, err)
	require.False(t, cfg.Enabled)
	require.Zero(t, cfg.Limit)
	require.Equal(t, "svc", cfg.Name)
}

// The environment still overrides the file.
func TestLoad_EnvOverridesFileAfterReapply(t *testing.T) {
	t.Setenv("EXPLICIT_LIMIT", "7")
	cfg := loadExplicit(t, "limit: 3\n")
	require.Equal(t, 7, cfg.Limit)
}

// Struct elements of slices and maps get defaults for omitted keys — including
// nested structs — and keep explicit zeros.
func TestLoad_CollectionElementsDefaultsAndExplicitZeros(t *testing.T) {
	cfg := loadExplicit(t, `
items:
  - name: a
  - name: b
    enabled: false
    weight: 0
pItems:
  - name: p
byName:
  c:
    name: c
    enabled: false
    inner:
      limit: 0
`)
	require.True(t, cfg.Items[0].Enabled)
	require.Equal(t, 10, cfg.Items[0].Weight)
	require.Equal(t, 5, cfg.Items[0].Inner.Limit)
	require.False(t, cfg.Items[1].Enabled)
	require.Zero(t, cfg.Items[1].Weight)

	require.True(t, cfg.PItems[0].Enabled)
	require.Equal(t, 10, cfg.PItems[0].Weight)

	c := cfg.ByName["c"]
	require.False(t, c.Enabled)
	require.Equal(t, 10, c.Weight)
	require.Zero(t, c.Inner.Limit)
	require.Equal(t, 15*time.Minute, c.Inner.Timeout)
}

// Files in a directory overlay in order; a later file recreating a pointer
// struct after an earlier null still gets defaults for omitted keys.
func TestLoad_DirectoryOverlayKeepsDefaults(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("ptr: null\nlimit: 3\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.yaml"), []byte("ptr:\n  enabled: false\nlimit: 0\n"), 0o600))

	cfg := &explicitConfig{}
	_, err := loader.New(nil, loader.WithPath(dir)).Load(cfg)
	require.NoError(t, err)
	require.Zero(t, cfg.Limit)
	require.NotNil(t, cfg.Ptr)
	require.False(t, cfg.Ptr.Enabled)
	require.Equal(t, 5, cfg.Ptr.Limit)
}

type tomlExplicitConfig struct {
	Enabled bool `toml:"enabled" default:"true"`
	Limit   int  `toml:"limit" default:"5"`
	Items   []struct {
		Name   string `toml:"name"`
		Weight int    `toml:"weight" default:"10"`
	} `toml:"items"`
}

// The TOML backend keys presence on its own struct tag.
func TestLoad_ExplicitZeroTOML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("enabled = false\n\n[[items]]\nname = \"a\"\n\n[[items]]\nname = \"b\"\nweight = 0\n"), 0o600))

	cfg := &tomlExplicitConfig{}
	_, err := loader.New(&toml.Backend{}, loader.WithPath(path)).Load(cfg)
	require.NoError(t, err)
	require.False(t, cfg.Enabled)
	require.Equal(t, 5, cfg.Limit)
	require.Equal(t, 10, cfg.Items[0].Weight)
	require.Zero(t, cfg.Items[1].Weight)
}
