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

// A reload discovers the directory afresh: a file removed since the previous
// load is neither opened nor applied.
func TestLoad_DirectoryReloadDropsRemovedFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("limit: 3\n"), 0o600))
	overlay := filepath.Join(dir, "b.yaml")
	require.NoError(t, os.WriteFile(overlay, []byte("name: overlay\n"), 0o600))

	l := loader.New(nil, loader.WithPath(dir))

	first := &explicitConfig{}
	_, err := l.Load(first)
	require.NoError(t, err)
	require.Equal(t, 3, first.Limit)
	require.Equal(t, "overlay", first.Name)

	require.NoError(t, os.Remove(overlay))

	second := &explicitConfig{}
	_, err = l.Load(second)
	require.NoError(t, err)
	require.Equal(t, 3, second.Limit)
	require.Equal(t, "svc", second.Name)
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

type sentinelItem struct {
	Name string          `yaml:"name"`
	Opt  *explicitNested `yaml:"opt" default:"-"`
}

type reviewConfig struct {
	Enabled bool                    `yaml:"enabled" default:"true"`
	List    []sentinelItem          `yaml:"list"`
	ByName  map[string]explicitItem `yaml:"byName"`
	Items   []explicitItem          `yaml:"items"`
}

func loadReview(t *testing.T, opts []loader.Option, files ...string) *reviewConfig {
	t.Helper()
	dir := t.TempDir()
	for i, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, string(rune('a'+i))+".yaml"), []byte(content), 0o600))
	}
	cfg := &reviewConfig{}
	_, err := loader.New(nil, append([]loader.Option{loader.WithPath(dir)}, opts...)...).Load(cfg)
	require.NoError(t, err)
	return cfg
}

// default:"-" on an element's optional pointer struct must not allocate it.
func TestLoad_ElementSentinelPointerStaysNil(t *testing.T) {
	cfg := loadReview(t, nil, "list:\n  - name: a\n")
	require.Nil(t, cfg.List[0].Opt)
}

// WithSkipDefaults also skips defaults for map and slice elements.
func TestLoad_SkipDefaultsCoversElements(t *testing.T) {
	cfg := loadReview(t, []loader.Option{loader.WithSkipDefaults()}, "items:\n  - name: a\nbyName:\n  b:\n    name: b\n")
	require.Zero(t, cfg.Items[0].Weight)
	require.Zero(t, cfg.ByName["b"].Weight)
	require.False(t, cfg.Enabled)
}

// A later file that rewrites a map entry replaces it, so keys only the
// earlier file set no longer suppress defaults.
func TestLoad_LaterFileReplacesMapEntryPresence(t *testing.T) {
	cfg := loadReview(t, nil, "byName:\n  a:\n    weight: 0\n", "byName:\n  a:\n    name: b\n")
	require.Equal(t, "b", cfg.ByName["a"].Name)
	require.Equal(t, 10, cfg.ByName["a"].Weight)
}

// YAML binds keys exactly: a differently cased key is ignored by the decoder
// and must not count as explicitly set.
func TestLoad_YAMLKeyCaseMustMatch(t *testing.T) {
	cfg := loadReview(t, []loader.Option{loader.WithAllowUnknownFields()}, "ENABLED: false\n")
	require.True(t, cfg.Enabled)
}

type hyphenItem struct {
	Label string `yaml:"label" default:"-"`
}

type keysConfig struct {
	Hyphens []hyphenItem            `yaml:"hyphens"`
	ByName  map[string]explicitItem `yaml:"byName"`
	Items   []explicitItem          `yaml:"items"`
}

func loadKeys(t *testing.T, content string, opts ...loader.Option) *keysConfig {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	cfg := &keysConfig{}
	_, err := loader.New(nil, append([]loader.Option{loader.WithPath(path)}, opts...)...).Load(cfg)
	require.NoError(t, err)
	return cfg
}

// On a scalar, default:"-" is an ordinary value, not the pointer sentinel.
func TestLoad_ElementScalarHyphenDefault(t *testing.T) {
	cfg := loadKeys(t, "hyphens:\n  - {}\n")
	require.Equal(t, "-", cfg.Hyphens[0].Label)
}

// Map keys keep their source spelling: "0x10" must not be recorded as "16".
func TestLoad_MapKeySpellingPreserved(t *testing.T) {
	cfg := loadKeys(t, "byName:\n  0x10:\n    weight: 0\n")
	require.Contains(t, cfg.ByName, "0x10")
	require.Zero(t, cfg.ByName["0x10"].Weight)
}

// Keys supplied through anchors and "<<" merges count as explicitly set.
func TestLoad_MergeKeysCountAsExplicit(t *testing.T) {
	// "base" binds to no field; it only holds the anchor.
	cfg := loadKeys(t, "base: &base\n  weight: 0\nitems:\n  - <<: *base\n    name: a\n", loader.WithAllowUnknownFields())
	require.Equal(t, "a", cfg.Items[0].Name)
	require.Zero(t, cfg.Items[0].Weight)
	require.True(t, cfg.Items[0].Enabled)
}

// A cyclic alias is rejected as a decode error instead of crashing the loader.
func TestLoad_CyclicAliasIsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("items: &x [*x]\n"), 0o600))
	_, err := loader.New(nil, loader.WithPath(path)).Load(&keysConfig{})
	require.ErrorIs(t, err, loader.ErrDecode)
}

type strictHostsConfig struct {
	Hosts []string `yaml:"hosts" env:"STRICT_HOSTS" default:"${STRICT_HOSTS_UNDEFINED_DEFAULT}"`
}

// A collection default that has no hole to fill is not substituted, so an
// undefined ${VAR} in it does not fail strict mode.
func TestLoad_StrictUnusedCollectionDefault(t *testing.T) {
	t.Setenv("STRICT_HOSTS__1", "replica:6379")

	cfg := &strictHostsConfig{Hosts: []string{"redis.internal:6379"}}
	_, err := loader.New(nil, loader.WithStrict()).Load(cfg)
	require.NoError(t, err)
	require.Equal(t, []string{"redis.internal:6379", "replica:6379"}, cfg.Hosts)
}
