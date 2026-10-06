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
	"github.com/altessa-s/go-atlas/config/loader/backend"
	"github.com/altessa-s/go-atlas/config/loader/backend/toml"
)

// loadFiles writes the named files into a temporary directory and loads cfg
// from it.
func loadFiles(t *testing.T, b backend.Backend, cfg any, files map[string]string, opts ...loader.Option) error {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
	}
	_, err := loader.New(b, append([]loader.Option{loader.WithPath(dir)}, opts...)...).Load(cfg)
	return err
}

type typedKeysConfig struct {
	ByID   map[int]explicitItem    `yaml:"byID"`
	ByFlag map[bool]explicitItem   `yaml:"byFlag"`
	ByName map[string]explicitItem `yaml:"byName"`
}

// Map entries bind by the key the decoder stores: YAML 0x10 is 16 in an
// int-keyed map and yes is true in a bool-keyed one, so their explicit zeros
// survive.
func TestLoad_TypedMapKeysKeepExplicitZeros(t *testing.T) {
	t.Parallel()

	t.Run("int key", func(t *testing.T) {
		t.Parallel()
		cfg := &typedKeysConfig{}
		require.NoError(t, loadFiles(t, nil, cfg, map[string]string{"a.yaml": "byID:\n  0x10: {weight: 0}\n"}))
		require.Contains(t, cfg.ByID, 16)
		require.Zero(t, cfg.ByID[16].Weight)
		require.True(t, cfg.ByID[16].Enabled, "omitted field still defaults")
	})
	t.Run("bool key", func(t *testing.T) {
		t.Parallel()
		cfg := &typedKeysConfig{}
		require.NoError(t, loadFiles(t, nil, cfg, map[string]string{"a.yaml": "byFlag:\n  yes: {enabled: false}\n"}))
		require.Contains(t, cfg.ByFlag, true)
		require.False(t, cfg.ByFlag[true].Enabled)
		require.Equal(t, 10, cfg.ByFlag[true].Weight)
	})
}

// yaml.v3 excludes "<<" keys by their resolved value: 16 does not exclude the
// string key "0x10", so the merged entry replaces the mapping's own and its
// explicit zero must survive.
func TestLoad_YAMLMergeResolvedKeyQuirk(t *testing.T) {
	t.Parallel()

	cfg := &typedKeysConfig{}
	require.NoError(t, loadFiles(t, nil, cfg, map[string]string{
		"a.yaml": "byName:\n  <<: {0x10: {name: merged, weight: 0}, a: {name: merged, weight: 0}}\n  0x10: {name: own}\n  a: {name: own}\n",
	}))
	require.Equal(t, "merged", cfg.ByName["0x10"].Name)
	require.Zero(t, cfg.ByName["0x10"].Weight)
	require.Equal(t, "own", cfg.ByName["a"].Name)
	require.Equal(t, 10, cfg.ByName["a"].Weight, "the merged weight does not apply to an entry the mapping overrides")
}

type defaulterNested struct {
	Port  int    `yaml:"port"`
	Proto string `yaml:"proto"`
}

func (n *defaulterNested) Default() {
	n.Port = 8080
	n.Proto = "tcp"
}

type defaulterItem struct {
	Name   string `yaml:"name"`
	Weight int    `yaml:"weight"`
}

func (i *defaulterItem) Default() {
	i.Weight = 10
}

type defaulterConfig struct {
	Enabled bool                     `yaml:"enabled" env:"DEFAULTER_TEST_ENABLED"`
	Limit   int                      `yaml:"limit"`
	Name    string                   `yaml:"name"`
	Nested  *defaulterNested         `yaml:"nested"`
	Items   []defaulterItem          `yaml:"items"`
	PItems  []*defaulterItem         `yaml:"pItems"`
	ByName  map[string]defaulterItem `yaml:"byName"`
}

func (c *defaulterConfig) Default() {
	c.Enabled = true
	c.Limit = 5
	c.Name = "svc"
}

// Default() fills what nothing set but never overrides an explicit value from
// a file or the environment — for the configuration, nested pointer structs
// and map and slice elements alike.
func TestLoad_DefaulterKeepsExplicitValues(t *testing.T) {
	t.Setenv("DEFAULTER_TEST_ENABLED", "false")

	cfg := &defaulterConfig{}
	require.NoError(t, loadFiles(t, nil, cfg, map[string]string{
		"a.yaml": `
limit: 0
nested:
  port: 0
items:
  - name: a
  - name: b
    weight: 0
pItems:
  - weight: 0
  - name: p
byName:
  m:
    weight: 0
  n: {}
`,
	}))

	t.Run("configuration", func(t *testing.T) {
		require.False(t, cfg.Enabled, "env wins over Default()")
		require.Zero(t, cfg.Limit, "file wins over Default()")
		require.Equal(t, "svc", cfg.Name, "Default() fills an omitted field")
	})
	t.Run("nested pointer struct", func(t *testing.T) {
		require.Zero(t, cfg.Nested.Port)
		require.Equal(t, "tcp", cfg.Nested.Proto)
	})
	t.Run("slice elements", func(t *testing.T) {
		require.Equal(t, 10, cfg.Items[0].Weight, "Default() runs on slice elements")
		require.Zero(t, cfg.Items[1].Weight)
		require.Zero(t, cfg.PItems[0].Weight)
		require.Equal(t, 10, cfg.PItems[1].Weight)
	})
	t.Run("map values", func(t *testing.T) {
		require.Zero(t, cfg.ByName["m"].Weight)
		require.Equal(t, 10, cfg.ByName["n"].Weight, "Default() runs on map values")
	})
}

type EmbeddedBase struct {
	Enabled bool `yaml:"enabled" toml:"enabled" default:"true"`
}

type yamlEmbeddedConfig struct {
	EmbeddedBase        // yaml.v3 binds it to the key "embeddedbase", not inline
	Name         string `yaml:"name"`
}

type yamlExcludedEmbedConfig struct {
	EmbeddedBase `yaml:"-"`
	Name         string `yaml:"name"`
}

type tomlNamedEmbedConfig struct {
	EmbeddedBase `toml:"base"`
	Name         string `toml:"name"`
}

// Anonymous fields follow each backend's embedding rules: yaml.v3 flattens
// only ",inline" and honors "-", BurntSushi/toml keys an embedded struct that
// has a tag name.
func TestLoad_EmbeddedStructsFollowBackend(t *testing.T) {
	t.Parallel()

	t.Run("yaml embedded without inline is keyed", func(t *testing.T) {
		t.Parallel()
		cfg := &yamlEmbeddedConfig{}
		require.NoError(t, loadFiles(t, nil, cfg, map[string]string{"a.yaml": "embeddedbase:\n  enabled: false\n"}))
		require.False(t, cfg.Enabled, "the explicit value under the embedded struct's key is kept")
	})
	t.Run("yaml excluded embedded", func(t *testing.T) {
		t.Parallel()
		cfg := &yamlExcludedEmbedConfig{}
		require.NoError(t, loadFiles(t, nil, cfg, map[string]string{"a.yaml": "enabled: false\n"},
			loader.WithAllowUnknownFields()))
		require.True(t, cfg.Enabled, "a key the decoder ignores does not suppress the default")
	})
	t.Run("toml embedded with tag name is keyed", func(t *testing.T) {
		t.Parallel()
		cfg := &tomlNamedEmbedConfig{}
		require.NoError(t, loadFiles(t, &toml.Backend{}, cfg, map[string]string{"a.toml": "[base]\nenabled = false\n"}))
		require.False(t, cfg.Enabled)
	})
}

type strictItem struct {
	Name string `yaml:"name"`
	Flag bool   `yaml:"flag" default:"${LOADER_PRESENCE_UNDEFINED}"`
}

type strictEnvSetConfig struct {
	Limit       int          `yaml:"limit" env:"PRESENCE_STRICT_LIMIT" default:"${LOADER_PRESENCE_UNDEFINED}"`
	StrictItems []strictItem `yaml:"strictItems"`
}

// In strict mode an undefined ${VAR} in a default tag fails only for a field
// that needs the default, not for one the environment sets.
func TestLoad_StrictDefaultSkippedForEnvSetFields(t *testing.T) {
	t.Setenv("PRESENCE_STRICT_LIMIT", "0")
	t.Setenv("STRICT_ITEMS__0__FLAG", "false")

	t.Run("set by the environment", func(t *testing.T) {
		cfg := &strictEnvSetConfig{}
		require.NoError(t, loadFiles(t, nil, cfg, map[string]string{"a.yaml": "strictItems:\n  - name: a\n"}, loader.WithStrict()))
		require.Zero(t, cfg.Limit)
		require.False(t, cfg.StrictItems[0].Flag)
	})
	t.Run("needing the default", func(t *testing.T) {
		cfg := &strictEnvSetConfig{}
		err := loadFiles(t, nil, cfg, map[string]string{"a.yaml": "strictItems:\n  - name: a\n  - name: b\n"}, loader.WithStrict())
		require.ErrorIs(t, err, loader.ErrUndefinedEnvVar, "an element the environment does not set still needs the default")
	})
}

type envMapItem struct {
	Name   string   `yaml:"name"`
	Weight int      `yaml:"weight"`
	Tags   []string `yaml:"tags"`
}

type envMapElement struct {
	ByKey map[string]envMapItem `yaml:"byKey"`
}

type envMapConfig struct {
	EnvByName map[string]envMapItem `yaml:"envByName"`
	EnvItems  []envMapElement       `yaml:"envItems"`
}

// The environment updates existing struct values of maps in place: an entry
// is copied, updated and stored back, keeping its other fields.
func TestLoad_EnvUpdatesExistingMapStructEntries(t *testing.T) {
	t.Setenv("ENV_BY_NAME__a__WEIGHT", "7")
	t.Setenv("ENV_BY_NAME__a__TAGS__0", "x")
	t.Setenv("ENV_ITEMS__0__BY_KEY__k__WEIGHT", "9")

	files := map[string]string{
		"a.yaml": "envByName:\n  a: {name: a, weight: 3}\nenvItems:\n  - byKey:\n      k: {name: k, weight: 3}\n",
	}
	cfg := &envMapConfig{}
	require.NoError(t, loadFiles(t, nil, cfg, files))

	t.Run("map field", func(t *testing.T) {
		require.Equal(t, "a", cfg.EnvByName["a"].Name)
		require.Equal(t, 7, cfg.EnvByName["a"].Weight)
	})
	t.Run("slice inside a map entry", func(t *testing.T) {
		require.Equal(t, []string{"x"}, cfg.EnvByName["a"].Tags)
	})
	t.Run("map inside a slice element", func(t *testing.T) {
		require.Equal(t, "k", cfg.EnvItems[0].ByKey["k"].Name)
		require.Equal(t, 9, cfg.EnvItems[0].ByKey["k"].Weight)
	})
	t.Run("strict", func(t *testing.T) {
		require.NoError(t, loadFiles(t, nil, &envMapConfig{}, files, loader.WithStrict()))
	})
}

type optionalItem struct {
	Name string          `yaml:"name"`
	Opt  *explicitNested `yaml:"opt"`
}

type optionalConfig struct {
	Optionals []optionalItem `yaml:"optionals"`
}

// A nil, untagged pointer struct inside an element is allocated and defaulted
// like one in the field list, unless the file set it to null.
func TestLoad_ElementNestedPointerAllocation(t *testing.T) {
	t.Parallel()

	cfg := &optionalConfig{}
	require.NoError(t, loadFiles(t, nil, cfg, map[string]string{
		"a.yaml": "optionals:\n  - name: a\n  - name: b\n    opt: null\n",
	}))

	t.Run("omitted", func(t *testing.T) {
		require.NotNil(t, cfg.Optionals[0].Opt)
		require.True(t, cfg.Optionals[0].Opt.Enabled)
		require.Equal(t, 5, cfg.Optionals[0].Opt.Limit)
	})
	t.Run("explicit null", func(t *testing.T) {
		require.Nil(t, cfg.Optionals[1].Opt, "an explicit null stays nil")
	})
}

type tomlItem struct {
	Name   string `toml:"name"`
	Weight int    `toml:"weight" default:"10"`
}

type tomlLimits struct {
	Limit int `toml:"limit" default:"5"`
}

type tomlFoldConfig struct {
	ByName map[string]tomlItem `toml:"byName"`
	Items  []tomlItem          `toml:"items"`
	Foo    tomlLimits
	FOO    tomlLimits
}

// TOML presence follows BurntSushi/toml's case folding: files spelling a field
// differently still overlay the same field, and a key binds to its exact
// field, never to a differently cased sibling.
func TestLoad_TOMLCaseFoldingFollowsDecoder(t *testing.T) {
	t.Parallel()

	t.Run("files spelling a field differently", func(t *testing.T) {
		t.Parallel()
		cfg := &tomlFoldConfig{}
		require.NoError(t, loadFiles(t, &toml.Backend{}, cfg, map[string]string{
			"a.toml": "[byName.a]\nweight = 0\n",
			"b.toml": "[ByName.a]\nname = \"b\"\n",
		}))
		require.Equal(t, "b", cfg.ByName["a"].Name)
		require.Equal(t, 10, cfg.ByName["a"].Weight, "b.toml replaces entry a, dropping a.toml's explicit weight")
	})
	t.Run("differently cased sibling", func(t *testing.T) {
		t.Parallel()
		cfg := &tomlFoldConfig{}
		require.NoError(t, loadFiles(t, &toml.Backend{}, cfg, map[string]string{"a.toml": "[Foo]\nlimit = 0\n"}))
		require.Zero(t, cfg.Foo.Limit)
		require.Equal(t, 5, cfg.FOO.Limit, "[Foo] does not set the sibling FOO")
	})
}

// BurntSushi/toml fills a slice from an earlier file in place when the later
// array is not longer, so an element keeps the explicit values of both files.
func TestLoad_TOMLSliceReuseKeepsEarlierExplicitValues(t *testing.T) {
	t.Parallel()

	cfg := &tomlFoldConfig{}
	require.NoError(t, loadFiles(t, &toml.Backend{}, cfg, map[string]string{
		"a.toml": "[[items]]\nname = \"a\"\nweight = 0\n\n[[items]]\nname = \"x\"\n",
		"b.toml": "[[items]]\nname = \"b\"\n",
	}))
	require.Len(t, cfg.Items, 1)
	require.Equal(t, "b", cfg.Items[0].Name)
	require.Zero(t, cfg.Items[0].Weight)
}

type mutatingItem struct {
	Name   string `yaml:"name"`
	Weight int    `yaml:"weight"`
}

type mutatingConfig struct {
	Count  *int                    `yaml:"count"`
	Items  []mutatingItem          `yaml:"items"`
	ByName map[string]mutatingItem `yaml:"byName"`
}

// Default mutates values the file set explicitly through shared pointers,
// slices and maps.
func (c *mutatingConfig) Default() {
	if c.Count != nil {
		*c.Count = 10
	}
	if len(c.Items) > 0 {
		c.Items[0].Weight = 10
	}
	if c.ByName != nil {
		c.ByName["a"] = mutatingItem{Name: "replacement", Weight: 10}
	}
}

// Explicit values survive a Default() that mutates them in place: the values
// restored afterwards are independent copies.
func TestLoad_DefaulterInPlaceMutation(t *testing.T) {
	t.Parallel()

	cfg := &mutatingConfig{}
	require.NoError(t, loadFiles(t, nil, cfg, map[string]string{
		"a.yaml": "count: 0\nitems:\n  - weight: 0\nbyName:\n  a:\n    weight: 0\n",
	}))

	t.Run("pointer", func(t *testing.T) {
		require.Zero(t, *cfg.Count)
	})
	t.Run("slice element", func(t *testing.T) {
		require.Zero(t, cfg.Items[0].Weight)
	})
	t.Run("map entry", func(t *testing.T) {
		require.Zero(t, cfg.ByName["a"].Weight)
		require.Equal(t, "replacement", cfg.ByName["a"].Name, "a field the file omitted keeps Default's value")
	})
}

type replacingConfig struct {
	Items  []mutatingItem    `yaml:"items"`
	Labels map[string]string `yaml:"labels"`
}

// Default replaces whole collections.
func (c *replacingConfig) Default() {
	c.Items = []mutatingItem{{Name: "default", Weight: 10}}
	c.Labels = map[string]string{"team": "core"}
}

// Only explicit descendants are restored after Default(): an element field the
// file omitted keeps Default's value, and a map entry set by the environment
// joins the map Default built.
func TestLoad_DefaulterReplacedCollections(t *testing.T) {
	t.Setenv("REPLACING_LABELS__env", "x")

	cfg := &replacingConfig{}
	require.NoError(t, loadFiles(t, nil, cfg, map[string]string{"a.yaml": "items:\n  - weight: 0\n"},
		loader.WithEnvPrefix("REPLACING_")))

	t.Run("slice", func(t *testing.T) {
		require.Len(t, cfg.Items, 1)
		require.Zero(t, cfg.Items[0].Weight)
		require.Equal(t, "default", cfg.Items[0].Name)
	})
	t.Run("map", func(t *testing.T) {
		require.Equal(t, map[string]string{"team": "core", "env": "x"}, cfg.Labels)
	})
}

type emptyEnvConfig struct {
	Enabled bool              `yaml:"enabled" env:"EMPTY_ENV_ENABLED" default:"true"`
	Limit   int               `yaml:"limit" env:"EMPTY_ENV_LIMIT" default:"5"`
	Ratio   float64           `yaml:"ratio" env:"EMPTY_ENV_RATIO" default:"0.5"`
	Timeout time.Duration     `yaml:"timeout" env:"EMPTY_ENV_TIMEOUT" default:"15m"`
	Hosts   []string          `yaml:"hosts" env:"EMPTY_ENV_HOSTS" default:"a,b"`
	Labels  map[string]string `yaml:"labels" env:"EMPTY_ENV_LABELS" default:"k:v"`
}

// An empty environment variable leaves a bool, number, duration, slice or map
// unassigned, so it does not count as an explicit value and the default still
// applies.
func TestLoad_EmptyEnvKeepsDefaults(t *testing.T) {
	for _, name := range []string{"ENABLED", "LIMIT", "RATIO", "TIMEOUT", "HOSTS", "LABELS"} {
		t.Setenv("EMPTY_ENV_"+name, "")
	}

	cfg := &emptyEnvConfig{}
	_, err := loader.New(nil).Load(cfg)
	require.NoError(t, err)

	tests := []struct {
		name string
		got  any
		want any
	}{
		{"bool", cfg.Enabled, true},
		{"int", cfg.Limit, 5},
		{"float", cfg.Ratio, 0.5},
		{"duration", cfg.Timeout, 15 * time.Minute},
		{"slice", cfg.Hosts, []string{"a", "b"}},
		{"map", cfg.Labels, map[string]string{"k": "v"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.got)
		})
	}
}

type nullEntryConfig struct {
	NullByName map[string]*explicitItem `yaml:"nullByName"`
}

// A null map entry replaces an earlier file's entry of pointer type, so its
// explicit values no longer suppress defaults once the entry is recreated.
func TestLoad_NullMapEntryReplacesPresence(t *testing.T) {
	t.Setenv("NULL_BY_NAME__a__NAME", "x")

	cfg := &nullEntryConfig{}
	require.NoError(t, loadFiles(t, nil, cfg, map[string]string{
		"a.yaml": "nullByName:\n  a: {weight: 0}\n",
		"b.yaml": "nullByName:\n  a: null\n",
	}))
	require.Equal(t, "x", cfg.NullByName["a"].Name)
	require.Equal(t, 10, cfg.NullByName["a"].Weight)
}

// BurntSushi/toml keeps a shrunk slice's backing array, so an element that
// comes back within capacity keeps an earlier file's explicit values.
func TestLoad_TOMLSliceShrinkThenGrow(t *testing.T) {
	t.Parallel()

	cfg := &tomlFoldConfig{}
	require.NoError(t, loadFiles(t, &toml.Backend{}, cfg, map[string]string{
		"a.toml": "[[items]]\nname = \"a\"\n\n[[items]]\nweight = 0\n",
		"b.toml": "[[items]]\nname = \"b\"\n",
		"c.toml": "[[items]]\nname = \"c\"\n\n[[items]]\nname = \"d\"\n",
	}))
	require.Len(t, cfg.Items, 2)
	require.Equal(t, "d", cfg.Items[1].Name)
	require.Zero(t, cfg.Items[1].Weight)
}

type TOMLEmbedded struct {
	Port int `toml:"port" default:"80"`
}

type tomlShadowNameConfig struct {
	TOMLEmbedded
	Embedded_0 int `toml:"e" default:"5"` //nolint:revive,staticcheck // collides with a synthetic shadow name
}

// TOML presence handles a field named like a synthetic shadow field.
func TestLoad_TOMLShadowFieldNames(t *testing.T) {
	t.Parallel()

	cfg := &tomlShadowNameConfig{}
	require.NoError(t, loadFiles(t, &toml.Backend{}, cfg, map[string]string{"a.toml": "port = 0\ne = 0\n"}))
	require.Zero(t, cfg.Port)
	require.Zero(t, cfg.Embedded_0)
}

type tomlDashConfig struct {
	Dash int `toml:"-,omitempty" default:"5"`
}

// TOML presence keeps the complete tag: "-,omitempty" is the literal key "-".
func TestLoad_TOMLDashKeyTag(t *testing.T) {
	t.Parallel()

	cfg := &tomlDashConfig{}
	require.NoError(t, loadFiles(t, &toml.Backend{}, cfg, map[string]string{"a.toml": "\"-\" = 0\n"}))
	require.Zero(t, cfg.Dash)
}

type envIndexConfig struct {
	EnvIndexHosts  []string          `yaml:"envIndexHosts" default:"a,b"`
	EnvIndexLabels map[string]string `yaml:"envIndexLabels" default:"k:v"`
}

// A default tag still provides the entries of a slice or map the environment
// populated entry by entry; the environment's entries win.
func TestLoad_DefaultTagFillsEnvCollections(t *testing.T) {
	t.Setenv("ENV_INDEX_HOSTS__1", "c")
	t.Setenv("ENV_INDEX_LABELS__x", "y")

	cfg := &envIndexConfig{}
	_, err := loader.New(nil).Load(cfg)
	require.NoError(t, err)
	require.Equal(t, []string{"a", "c"}, cfg.EnvIndexHosts)
	require.Equal(t, map[string]string{"k": "v", "x": "y"}, cfg.EnvIndexLabels)
}

type preloadedConfig struct {
	PreHosts  []string          `yaml:"preHosts" default:"localhost:6379"`
	PreLabels map[string]string `yaml:"preLabels" default:"k:v"`
}

// Indexed environment overrides extend a slice or map passed to Load without
// the default tag overwriting or adding to its existing entries.
func TestLoad_EnvIndexKeepsPreloadedEntries(t *testing.T) {
	t.Setenv("PRE_HOSTS__1", "replica:6379")
	t.Setenv("PRE_LABELS__x", "y")

	cfg := &preloadedConfig{
		PreHosts:  []string{"redis.internal:6379"},
		PreLabels: map[string]string{"team": "core"},
	}
	_, err := loader.New(nil).Load(cfg)
	require.NoError(t, err)

	t.Run("slice", func(t *testing.T) {
		require.Equal(t, []string{"redis.internal:6379", "replica:6379"}, cfg.PreHosts)
	})
	t.Run("map", func(t *testing.T) {
		require.Equal(t, map[string]string{"team": "core", "x": "y"}, cfg.PreLabels)
	})
}

type hookHostsConfig struct {
	HookHosts []string `yaml:"hookHosts" default:"a,b"`
}

// Default fills the empty hosts.
func (c *hookHostsConfig) Default() {
	for i, h := range c.HookHosts {
		if h == "" {
			c.HookHosts[i] = "hook"
		}
	}
}

// A default tag does not overwrite an entry Default() supplied in a slice the
// environment grew.
func TestLoad_EnvIndexKeepsDefaulterEntries(t *testing.T) {
	t.Setenv("HOOK_HOSTS__1", "c")

	cfg := &hookHostsConfig{}
	_, err := loader.New(nil).Load(cfg)
	require.NoError(t, err)
	require.Equal(t, []string{"hook", "c"}, cfg.HookHosts)
}
