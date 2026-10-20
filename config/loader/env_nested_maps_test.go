// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package loader_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config/loader"
)

// All tests in this file mutate process environment via t.Setenv, which is
// incompatible with t.Parallel — keep them serial.

type nestedMapsSection struct {
	Name   string                               `yaml:"name"`
	Labels map[string]map[string]string         `yaml:"labels"`
	Deep   map[string]map[string]map[string]int `yaml:"deep"`
	Tagged map[string]map[string]string         `yaml:"tagged" env:"NM_SECTION_TAGGED"`
	Items  []string                             `yaml:"items"`
	ByName map[string]nestedMapsEntry           `yaml:"byName"`
	Counts map[string]map[int]string            `yaml:"counts"`
}

type nestedMapsEntry struct {
	Name  string   `yaml:"name"`
	Hosts []string `yaml:"hosts"`
}

type nestedMapsConfig struct {
	Labels  map[string]map[string]string `yaml:"labels"`
	Tagged  map[string]map[string]string `yaml:"tagged" env:"NM_TAGGED"`
	Section *nestedMapsSection           `yaml:"section" default:"-"`
	Counts  map[string]map[int]string    `yaml:"counts"`
}

func loadNestedMaps(t testing.TB, cfg *nestedMapsConfig, opts ...loader.Option) *nestedMapsConfig {
	t.Helper()
	_, err := loader.New(nil, append([]loader.Option{loader.WithEnvPrefix("NMAP")}, opts...)...).Load(cfg)
	require.NoError(t, err)
	return cfg
}

// TestLoad_EnvMapOfMapsEntries checks that a scalar entry of a map of maps is
// set on the untagged and the tagged path, creating the outer and inner maps
// or adding to the existing ones.
func TestLoad_EnvMapOfMapsEntries(t *testing.T) {
	tests := []struct {
		name   string
		varSet string
		get    func(*nestedMapsConfig) map[string]map[string]string
		preset func(*nestedMapsConfig, map[string]map[string]string)
	}{
		{
			name:   "untagged",
			varSet: "NMAP_LABELS",
			get:    func(c *nestedMapsConfig) map[string]map[string]string { return c.Labels },
			preset: func(c *nestedMapsConfig, m map[string]map[string]string) { c.Labels = m },
		},
		{
			name:   "tagged",
			varSet: "NMAP_NM_TAGGED",
			get:    func(c *nestedMapsConfig) map[string]map[string]string { return c.Tagged },
			preset: func(c *nestedMapsConfig, m map[string]map[string]string) { c.Tagged = m },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name+"/new keys", func(t *testing.T) {
			t.Setenv(tt.varSet+"__team__owner", "radar")
			t.Setenv(tt.varSet+"__team__tier", "1")

			cfg := loadNestedMaps(t, &nestedMapsConfig{})
			require.Equal(t, map[string]map[string]string{"team": {"owner": "radar", "tier": "1"}}, tt.get(cfg))
		})
		t.Run(tt.name+"/existing keys", func(t *testing.T) {
			t.Setenv(tt.varSet+"__team__owner", "radar")
			t.Setenv(tt.varSet+"__other__k", "v")

			cfg := &nestedMapsConfig{}
			tt.preset(cfg, map[string]map[string]string{"team": {"owner": "old", "keep": "x"}})
			loadNestedMaps(t, cfg)
			require.Equal(t, map[string]map[string]string{
				"team":  {"owner": "radar", "keep": "x"},
				"other": {"k": "v"},
			}, tt.get(cfg))
		})
	}
}

// TestLoad_EnvMapOfMapsInSection checks that map of maps entries, also three
// levels deep, are set inside a nil pointer section, which they allocate.
func TestLoad_EnvMapOfMapsInSection(t *testing.T) {
	t.Setenv("NMAP_SECTION__LABELS__a__b", "v")
	t.Setenv("NMAP_SECTION__DEEP__x__y__z", "3")
	t.Setenv("NMAP_NM_SECTION_TAGGED__p__q", "w")

	cfg := loadNestedMaps(t, &nestedMapsConfig{})
	require.NotNil(t, cfg.Section)
	require.Equal(t, map[string]map[string]string{"a": {"b": "v"}}, cfg.Section.Labels)
	require.Equal(t, map[string]map[string]map[string]int{"x": {"y": {"z": 3}}}, cfg.Section.Deep)
	require.Equal(t, map[string]map[string]string{"p": {"q": "w"}}, cfg.Section.Tagged)
}

// TestLoad_EnvMapOfMapsCustomDelimiter checks map of maps entries with a
// custom section delimiter.
func TestLoad_EnvMapOfMapsCustomDelimiter(t *testing.T) {
	t.Setenv("NMAP_LABELS--team--owner", "radar")
	t.Setenv("NMAP_NM_TAGGED--team--tier", "1")
	t.Setenv("NMAP_SECTION--DEEP--x--y--z", "3")

	cfg := loadNestedMaps(t, &nestedMapsConfig{}, loader.WithEnvSectionDelimiter("--"))
	require.Equal(t, map[string]map[string]string{"team": {"owner": "radar"}}, cfg.Labels)
	require.Equal(t, map[string]map[string]string{"team": {"tier": "1"}}, cfg.Tagged)
	require.NotNil(t, cfg.Section)
	require.Equal(t, map[string]map[string]map[string]int{"x": {"y": {"z": 3}}}, cfg.Section.Deep)
}

type nestedMapsKey string

type nestedMapsItem struct {
	Labels map[string]map[string]string `yaml:"labels"`
	Counts map[string]map[int]string    `yaml:"counts"`
	ByID   map[string]string            `yaml:"byID"`
	Named  map[nestedMapsKey]string     `yaml:"named"`
}

type nestedMapsItems struct {
	Items []nestedMapsItem `yaml:"items"`
}

// TestLoad_EnvMapOfMapsInSliceElement checks that map of maps entries are set
// inside slice elements too.
func TestLoad_EnvMapOfMapsInSliceElement(t *testing.T) {
	t.Setenv("NMAP_ITEMS__0__LABELS__a__b", "v")
	t.Setenv("NMAP_ITEMS__0__LABELS__a__c", "w")

	cfg := &nestedMapsItems{}
	_, err := loader.New(nil, loader.WithEnvPrefix("NMAP")).Load(cfg)
	require.NoError(t, err)
	require.Len(t, cfg.Items, 1)
	require.Equal(t, map[string]map[string]string{"a": {"b": "v", "c": "w"}}, cfg.Items[0].Labels)
}

// TestLoad_EnvNonStringMapKeys checks that a variable addressing a map whose
// keys are not strings, at the top level or inside a slice element, sets and
// allocates nothing instead of panicking.
func TestLoad_EnvNonStringMapKeys(t *testing.T) {
	t.Run("top level", func(t *testing.T) {
		t.Setenv("NMAP_COUNTS__team__1", "v")

		cfg := loadNestedMaps(t, &nestedMapsConfig{})
		require.Nil(t, cfg.Counts)
	})
	t.Run("slice element", func(t *testing.T) {
		t.Setenv("NMAP_ITEMS__0__COUNTS__team__1", "v")

		cfg := &nestedMapsItems{}
		require.NotPanics(t, func() {
			_, err := loader.New(nil, loader.WithEnvPrefix("NMAP")).Load(cfg)
			require.NoError(t, err)
		})
		require.Empty(t, cfg.Items)
	})
}

// TestLoad_EnvIndexedPathsStillResolve checks that the indexed paths the
// type checks accept are set: a numeric map key inside a slice element, a
// map key of a named string type and a slice inside a map value.
func TestLoad_EnvIndexedPathsStillResolve(t *testing.T) {
	t.Setenv("NMAP_ITEMS__0__BY_ID__5", "five")
	t.Setenv("NMAP_ITEMS__0__NAMED__k", "v")

	items := &nestedMapsItems{}
	_, err := loader.New(nil, loader.WithEnvPrefix("NMAP")).Load(items)
	require.NoError(t, err)
	require.Len(t, items.Items, 1)
	require.Equal(t, map[string]string{"5": "five"}, items.Items[0].ByID)
	require.Equal(t, map[nestedMapsKey]string{"k": "v"}, items.Items[0].Named)

	t.Setenv("NMAP_SECTION__BY_NAME__a__HOSTS__0", "h0")

	cfg := loadNestedMaps(t, &nestedMapsConfig{})
	require.NotNil(t, cfg.Section)
	require.Equal(t, []string{"h0"}, cfg.Section.ByName["a"].Hosts)
}

// TestLoad_EnvStrictUnknownElementField checks that in strict mode an
// unknown field inside a slice element still fails the load.
func TestLoad_EnvStrictUnknownElementField(t *testing.T) {
	t.Setenv("NMAP_ITEMS__0__TYPO", "v")

	_, err := loader.New(nil, loader.WithEnvPrefix("NMAP"), loader.WithStrict()).Load(&nestedMapsItems{})
	require.ErrorIs(t, err, loader.ErrFieldNotFound)
}

// TestLoad_EnvUnresolvedPathKeepsNilSection checks that a variable naming a
// nil pointer section but nothing settable in it leaves the section nil, on
// the untagged and the tagged path, while a valid one allocates and sets it.
func TestLoad_EnvUnresolvedPathKeepsNilSection(t *testing.T) {
	unresolved := []string{
		"NMAP_SECTION__TYPO",                // no such field
		"NMAP_SECTION__NAME__EXTRA",         // a level below a value
		"NMAP_SECTION__LABELS__a__b__c",     // deeper than the map of maps
		"NMAP_SECTION__DEEP__x__y__z__w",    // deeper than the nested maps
		"NMAP_SECTION__ITEMS__x",            // not a slice index
		"NMAP_NM_SECTION_TAGGED__p__q__r",   // tagged, deeper than the map of maps
		"NMAP_SECTION__TAGGED__p__q",        // the tag names the field
		"NMAP_SECTION__LABELS__a__b__c__d", // deeper still
		"NMAP_SECTION__NAME__0",             // an index below a value
		"NMAP_SECTION__ITEMS__0__x",         // a level below a slice element value
		"NMAP_SECTION__BY_NAME__5__TYPO",     // numeric map key, no such field
		"NMAP_SECTION__BY_NAME__5__NAME__0",  // numeric map key, an index below a value
		"NMAP_SECTION__BY_NAME__a__HOSTS__x", // not a slice index in a map value
		"NMAP_SECTION__COUNTS__a__1",        // map keys that are not strings
	}
	for _, name := range unresolved {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, "v")

			cfg := loadNestedMaps(t, &nestedMapsConfig{})
			require.Nil(t, cfg.Section)
		})
	}

	t.Run("valid", func(t *testing.T) {
		t.Setenv("NMAP_SECTION__TYPO", "v")
		t.Setenv("NMAP_SECTION__NAME", "svc")

		cfg := loadNestedMaps(t, &nestedMapsConfig{})
		require.NotNil(t, cfg.Section)
		require.Equal(t, "svc", cfg.Section.Name)
	})
}

type mapKeysEntry struct {
	Name  string `yaml:"name"`
	Count int    `yaml:"count" default:"7"`
}

type mapKeysItem struct {
	Labels map[string]map[string]mapKeysEntry `yaml:"labels"`
	Named  map[nestedMapsKey]mapKeysEntry     `yaml:"named"`
}

type mapKeysConfig struct {
	Labels map[string]map[string]mapKeysEntry `yaml:"labels"`
	Named  map[nestedMapsKey]mapKeysEntry     `yaml:"named"`
	Items  []mapKeysItem                      `yaml:"items"`
}

// TestLoad_EnvStrictUnknownFieldBelowNestedMaps checks that in strict mode an
// unknown struct field below nested maps fails the load, at the top level and
// inside a slice element.
func TestLoad_EnvStrictUnknownFieldBelowNestedMaps(t *testing.T) {
	for _, name := range []string{"NMAP_LABELS__a__b__TYPO", "NMAP_ITEMS__0__LABELS__a__b__TYPO"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, "v")

			_, err := loader.New(nil, loader.WithEnvPrefix("NMAP"), loader.WithStrict()).Load(&mapKeysConfig{})
			require.ErrorIs(t, err, loader.ErrFieldNotFound)
		})
	}
}

// TestLoad_EnvNamedMapKeyExplicitZero checks that an explicit zero the
// environment sets beneath a map key of a named string type survives the
// default tag, at the top level and inside a slice element, while the default
// still fills the fields it did not set.
func TestLoad_EnvNamedMapKeyExplicitZero(t *testing.T) {
	t.Setenv("NMAP_NAMED__k__COUNT", "0")
	t.Setenv("NMAP_NAMED__d__NAME", "x")
	t.Setenv("NMAP_ITEMS__0__NAMED__k__COUNT", "0")
	t.Setenv("NMAP_ITEMS__0__NAMED__d__NAME", "x")

	cfg := &mapKeysConfig{}
	_, err := loader.New(nil, loader.WithEnvPrefix("NMAP")).Load(cfg)
	require.NoError(t, err)
	require.Equal(t, map[nestedMapsKey]mapKeysEntry{"k": {Count: 0}, "d": {Name: "x", Count: 7}}, cfg.Named)
	require.Len(t, cfg.Items, 1)
	require.Equal(t, map[nestedMapsKey]mapKeysEntry{"k": {Count: 0}, "d": {Name: "x", Count: 7}}, cfg.Items[0].Named)
}
