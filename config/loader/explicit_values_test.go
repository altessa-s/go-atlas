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
)

type explicitNested struct {
	Enabled bool          `yaml:"enabled" default:"true"`
	Limit   int           `yaml:"limit" default:"5"`
	Timeout time.Duration `yaml:"timeout" default:"15m"`
}

type explicitItem struct {
	Name    string `yaml:"name"`
	Enabled bool   `yaml:"enabled" default:"true"`
	Weight  int    `yaml:"weight" default:"10"`
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
