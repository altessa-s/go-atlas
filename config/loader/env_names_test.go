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

type namesMongo struct {
	Database string `yaml:"database"`
	Host     string `yaml:"host" env:"NAMES_MONGO_HOST"`
}

type namesItem struct {
	Weight int `yaml:"weight" env:"NAMES_ITEM_WEIGHT"`
}

type namesConfig struct {
	MaxConns int         `yaml:"maxconns"`
	Home     string      `yaml:"home" env:"SERVICE_HOME"`
	MongoDB  namesMongo  `yaml:"mongodb"`
	Store    *namesMongo `yaml:"filestore" env:"NAMES_STORE"`
	Items    []namesItem `yaml:"items"`
}

func loadNames(t testing.TB) *namesConfig {
	t.Helper()
	cfg := &namesConfig{}
	_, err := loader.New(nil, loader.WithEnvPrefix("ENVNAME")).Load(cfg)
	require.NoError(t, err)
	return cfg
}

// TestLoad_EnvLevelNames checks that every level is named the same way on
// every path: by its struct tag or its Go field name, in SCREAMING_SNAKE_CASE.
func TestLoad_EnvLevelNames(t *testing.T) {
	tests := []struct {
		name      string
		maxConns  string
		mongo     string
		fileStore string
	}{
		{name: "struct tags", maxConns: "MAXCONNS", mongo: "MONGODB", fileStore: "FILESTORE"},
		{name: "field names", maxConns: "MAX_CONNS", mongo: "MONGO_DB", fileStore: "STORE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ENVNAME_"+tt.maxConns, "7")
			t.Setenv("ENVNAME_"+tt.mongo+"__DATABASE", "radar")
			t.Setenv("ENVNAME_"+tt.fileStore+"__DATABASE", "files")

			cfg := loadNames(t)
			require.Equal(t, 7, cfg.MaxConns)
			require.Equal(t, "radar", cfg.MongoDB.Database)
			require.NotNil(t, cfg.Store)
			require.Equal(t, "files", cfg.Store.Database)
		})
	}
}

// TestLoad_EnvTagReplacesPathName checks that a field with an env tag is
// read only from the prefix followed by the tag, not from its path name.
func TestLoad_EnvTagReplacesPathName(t *testing.T) {
	t.Setenv("ENVNAME_HOME", "leaked")
	t.Setenv("ENVNAME_MONGODB__HOST", "leaked")

	cfg := loadNames(t)
	require.Empty(t, cfg.Home)
	require.Empty(t, cfg.MongoDB.Host)

	t.Setenv("ENVNAME_SERVICE_HOME", "/srv")
	t.Setenv("ENVNAME_NAMES_MONGO_HOST", "mongo.internal")

	cfg = loadNames(t)
	require.Equal(t, "/srv", cfg.Home)
	require.Equal(t, "mongo.internal", cfg.MongoDB.Host)
}

// TestLoad_EnvTagIsNotLevelName checks that the env tag of a section does not
// name it in the variables of its fields.
func TestLoad_EnvTagIsNotLevelName(t *testing.T) {
	t.Setenv("ENVNAME_NAMES_STORE__DATABASE", "leaked")

	cfg := loadNames(t)
	if cfg.Store != nil {
		require.Empty(t, cfg.Store.Database)
	}
}

// TestLoad_EnvTagInsideSliceElement checks that an element field keeps its
// path name: an env tag cannot name a field of one element.
func TestLoad_EnvTagInsideSliceElement(t *testing.T) {
	t.Setenv("ENVNAME_ITEMS__0__WEIGHT", "3")

	cfg := loadNames(t)
	require.Len(t, cfg.Items, 1)
	require.Equal(t, 3, cfg.Items[0].Weight)
}

// TestLoad_EnvNamesAreUpperCase checks that level names match only in
// SCREAMING_SNAKE_CASE.
func TestLoad_EnvNamesAreUpperCase(t *testing.T) {
	t.Setenv("ENVNAME_maxconns", "7")
	t.Setenv("ENVNAME_mongodb__database", "leaked")

	cfg := loadNames(t)
	require.Zero(t, cfg.MaxConns)
	require.Empty(t, cfg.MongoDB.Database)
}

type namesTaggedCollections struct {
	Labels map[string]string `yaml:"labels" env:"NAMES_LABELS"`
	Hosts  []string          `yaml:"hosts" env:"NAMES_HOSTS"`
}

// TestLoad_EnvTaggedCollectionEntries checks that entries of a slice or map
// field with an env tag are addressed through the tag, not the path name.
func TestLoad_EnvTaggedCollectionEntries(t *testing.T) {
	t.Setenv("ENVNAME_NAMES_LABELS__k", "v")
	t.Setenv("ENVNAME_NAMES_HOSTS__0", "a")
	t.Setenv("ENVNAME_LABELS__leaked", "x")
	t.Setenv("ENVNAME_HOSTS__1", "leaked")

	cfg := &namesTaggedCollections{}
	_, err := loader.New(nil, loader.WithEnvPrefix("ENVNAME")).Load(cfg)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"k": "v"}, cfg.Labels)
	require.Equal(t, []string{"a"}, cfg.Hosts)
}

type namesSection struct {
	Hosts []string `yaml:"hosts" env:"NAMES_SECTION_HOSTS"`
}

type namesNilSectionConfig struct {
	Section *namesSection `yaml:"section"`
}

// TestLoad_EnvTaggedSliceUnderNilSection checks that indexed entries of a
// tagged slice reach it when its section starts as a nil pointer.
func TestLoad_EnvTaggedSliceUnderNilSection(t *testing.T) {
	t.Setenv("ENVNAME_NAMES_SECTION_HOSTS__0", "a")
	t.Setenv("ENVNAME_NAMES_SECTION_HOSTS__1", "b")

	cfg := &namesNilSectionConfig{}
	_, err := loader.New(nil, loader.WithEnvPrefix("ENVNAME")).Load(cfg)
	require.NoError(t, err)
	require.NotNil(t, cfg.Section)
	require.Equal(t, []string{"a", "b"}, cfg.Section.Hosts)
}

type namesGroup struct {
	Name  string   `yaml:"name"`
	Hosts []string `yaml:"hosts"`
}

type namesGroupsConfig struct {
	Groups  map[string]namesGroup  `yaml:"groups" env:"NAMES_CUSTOM_GROUPS"`
	PGroups map[string]*namesGroup `yaml:"pGroups" env:"NAMES_CUSTOM_PGROUPS"`
	Lists   []namesGroup           `yaml:"lists" env:"NAMES_CUSTOM_LISTS"`
}

// TestLoad_EnvTaggedCollectionNestedIndexes checks that indexed entries reach
// slices nested in the values and elements of a tagged collection, for new
// and existing entries.
func TestLoad_EnvTaggedCollectionNestedIndexes(t *testing.T) {
	t.Setenv("ENVNAME_NAMES_CUSTOM_GROUPS__east__HOSTS__0", "a")
	t.Setenv("ENVNAME_NAMES_CUSTOM_GROUPS__west__HOSTS__1", "w")
	t.Setenv("ENVNAME_NAMES_CUSTOM_PGROUPS__east__HOSTS__0", "p")
	t.Setenv("ENVNAME_NAMES_CUSTOM_LISTS__0__HOSTS__0", "l")

	cfg := &namesGroupsConfig{
		Groups:  map[string]namesGroup{"west": {Name: "w", Hosts: []string{"old0", "old1"}}},
		PGroups: map[string]*namesGroup{},
		Lists:   []namesGroup{{Name: "first"}},
	}
	_, err := loader.New(nil, loader.WithEnvPrefix("ENVNAME")).Load(cfg)
	require.NoError(t, err)
	require.Equal(t, []string{"a"}, cfg.Groups["east"].Hosts)
	require.Equal(t, namesGroup{Name: "w", Hosts: []string{"old0", "w"}}, cfg.Groups["west"])
	require.NotNil(t, cfg.PGroups["east"])
	require.Equal(t, []string{"p"}, cfg.PGroups["east"].Hosts)
	require.Equal(t, []namesGroup{{Name: "first", Hosts: []string{"l"}}}, cfg.Lists)
}

type namesNilGroup struct {
	Name  string   `yaml:"name" default:"unnamed"`
	Hosts []string `yaml:"hosts"`
}

type namesNilEntriesConfig struct {
	PGroups map[string]*namesNilGroup `yaml:"pGroups" env:"NAMES_NIL_PGROUPS"`
	Groups  map[string]*namesNilGroup `yaml:"groups"`
	PLists  []*namesNilGroup          `yaml:"pLists"`
}

// TestLoad_EnvNilPointerEntries checks that indexed variables reach a slice
// inside an existing nil pointer map entry or slice element: the entry is
// allocated, the variable sets its element, and the default tag fills the
// fields nothing set.
func TestLoad_EnvNilPointerEntries(t *testing.T) {
	t.Setenv("ENVNAME_NAMES_NIL_PGROUPS__east__HOSTS__0", "p")
	t.Setenv("ENVNAME_GROUPS__east__HOSTS__0", "g")
	t.Setenv("ENVNAME_P_LISTS__0__HOSTS__0", "l")

	cfg := &namesNilEntriesConfig{
		PGroups: map[string]*namesNilGroup{"east": nil},
		Groups:  map[string]*namesNilGroup{"east": nil},
		PLists:  []*namesNilGroup{nil},
	}
	_, err := loader.New(nil, loader.WithEnvPrefix("ENVNAME")).Load(cfg)
	require.NoError(t, err)
	want := func(host string) *namesNilGroup { return &namesNilGroup{Name: "unnamed", Hosts: []string{host}} }
	require.Equal(t, want("p"), cfg.PGroups["east"])
	require.Equal(t, want("g"), cfg.Groups["east"])
	require.Equal(t, []*namesNilGroup{want("l")}, cfg.PLists)
}

type namesEntry struct {
	Name string `yaml:"name"`
}

type namesMapSlicesConfig struct {
	Groups   map[string][]string            `yaml:"groups" env:"NAMES_CUSTOM_GROUPS"`
	Plain    map[string][]string            `yaml:"plain"`
	Entries  map[string][]namesEntry        `yaml:"entries" env:"NAMES_CUSTOM_ENTRIES"`
	Nested   map[string]map[string][]string `yaml:"nested" env:"NAMES_CUSTOM_NESTED"`
	PEntries map[string]*[]string           `yaml:"pEntries"`
}

// TestLoad_EnvIndexedMapSliceValues checks that indexed variables grow a
// slice held directly as a map value, for new and existing entries, with and
// without an env tag, including struct elements and maps of maps.
func TestLoad_EnvIndexedMapSliceValues(t *testing.T) {
	t.Setenv("ENVNAME_NAMES_CUSTOM_GROUPS__east__0", "a")
	t.Setenv("ENVNAME_NAMES_CUSTOM_GROUPS__west__2", "w2")
	t.Setenv("ENVNAME_PLAIN__north__1", "n1")
	t.Setenv("ENVNAME_NAMES_CUSTOM_ENTRIES__east__1__NAME", "e1")
	t.Setenv("ENVNAME_NAMES_CUSTOM_NESTED__a__b__0", "ab")
	t.Setenv("ENVNAME_NAMES_CUSTOM_NESTED__old__b__1", "old1")
	t.Setenv("ENVNAME_P_ENTRIES__p__0", "p0")

	cfg := &namesMapSlicesConfig{
		Groups: map[string][]string{"west": {"w0"}},
		Nested: map[string]map[string][]string{"old": {"b": {"old0"}}},
	}
	_, err := loader.New(nil, loader.WithEnvPrefix("ENVNAME")).Load(cfg)
	require.NoError(t, err)
	require.Equal(t, map[string][]string{"east": {"a"}, "west": {"w0", "", "w2"}}, cfg.Groups)
	require.Equal(t, map[string][]string{"north": {"", "n1"}}, cfg.Plain)
	require.Equal(t, map[string][]namesEntry{"east": {{}, {Name: "e1"}}}, cfg.Entries)
	require.Equal(t, map[string]map[string][]string{
		"a":   {"b": {"ab"}},
		"old": {"b": {"old0", "old1"}},
	}, cfg.Nested)
	require.NotNil(t, cfg.PEntries["p"])
	require.Equal(t, []string{"p0"}, *cfg.PEntries["p"])
}

type namesDelimTagConfig struct {
	Labels map[string]string   `yaml:"labels" env:"DB__LABELS"`
	Groups map[string][]string `yaml:"groups" env:"DB__GROUPS"`
	Hosts  []string            `yaml:"hosts" env:"DB__HOSTS"`
}

type namesDashTagConfig struct {
	Labels map[string]string   `yaml:"labels" env:"DB--LABELS"`
	Groups map[string][]string `yaml:"groups" env:"DB--GROUPS"`
	Hosts  []string            `yaml:"hosts" env:"DB--HOSTS"`
}

// TestLoad_EnvTagWithDelimiter checks that a collection env tag containing
// the section delimiter addresses the entries of its field, with the default
// and a custom delimiter.
func TestLoad_EnvTagWithDelimiter(t *testing.T) {
	tests := []struct {
		name  string
		delim string
		load  func(t testing.TB) namesDelimTagConfig
	}{
		{
			name:  "default delimiter",
			delim: loader.DefaultEnvSectionDelimiter,
			load: func(t testing.TB) namesDelimTagConfig {
				t.Helper()
				cfg := &namesDelimTagConfig{}
				_, err := loader.New(nil, loader.WithEnvPrefix("ENVNAME")).Load(cfg)
				require.NoError(t, err)
				return *cfg
			},
		},
		{
			name:  "custom delimiter",
			delim: "--",
			load: func(t testing.TB) namesDelimTagConfig {
				t.Helper()
				cfg := &namesDashTagConfig{}
				_, err := loader.New(nil, loader.WithEnvPrefix("ENVNAME"), loader.WithEnvSectionDelimiter("--")).Load(cfg)
				require.NoError(t, err)
				return namesDelimTagConfig(*cfg)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := tt.delim
			t.Setenv("ENVNAME_DB"+d+"LABELS"+d+"team", "core")
			t.Setenv("ENVNAME_DB"+d+"GROUPS"+d+"east"+d+"1", "e1")
			t.Setenv("ENVNAME_DB"+d+"HOSTS"+d+"0", "h0")

			cfg := tt.load(t)
			require.Equal(t, map[string]string{"team": "core"}, cfg.Labels)
			require.Equal(t, map[string][]string{"east": {"", "e1"}}, cfg.Groups)
			require.Equal(t, []string{"h0"}, cfg.Hosts)
		})
	}
}

type namesClaimEntry struct {
	K string `yaml:"k"`
}

type namesClaimConfig struct {
	Col   map[string]namesClaimEntry `yaml:"col" env:"NAMES_COL"`
	Sub   map[string]string          `yaml:"sub" env:"NAMES_COL__SUB"`
	Hosts []string                   `yaml:"hosts" env:"NAMES_COL__HOSTS"`
}

// TestLoad_EnvTagLongestClaimWins checks the collision policy between tags
// that are prefixes of one another: the longest tag claiming a variable wins,
// a tag naming the whole variable included.
func TestLoad_EnvTagLongestClaimWins(t *testing.T) {
	t.Setenv("ENVNAME_NAMES_COL__SUB__K", "sub")
	t.Setenv("ENVNAME_NAMES_COL__east__K", "east")
	t.Setenv("ENVNAME_NAMES_COL__HOSTS", "a,b")

	cfg := &namesClaimConfig{}
	_, err := loader.New(nil, loader.WithEnvPrefix("ENVNAME")).Load(cfg)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"K": "sub"}, cfg.Sub)
	require.Equal(t, map[string]namesClaimEntry{"east": {K: "east"}}, cfg.Col)
	require.Equal(t, []string{"a", "b"}, cfg.Hosts)
}

type namesDBSection struct {
	Host string `yaml:"host" env:"NAMES_DB_HOST"`
}

type namesScalarTagConfig struct {
	DB *namesDBSection `yaml:"db" default:"-"`
}

// TestLoad_EnvScalarTagHasNoEntries checks that a variable naming a descendant
// of a tagged scalar is ignored without allocating the nil section holding it.
func TestLoad_EnvScalarTagHasNoEntries(t *testing.T) {
	t.Setenv("ENVNAME_NAMES_DB_HOST__TYPO", "value")
	t.Setenv("ENVNAME_NAMES_DB_HOST__0", "value")

	cfg := &namesScalarTagConfig{}
	_, err := loader.New(nil, loader.WithEnvPrefix("ENVNAME")).Load(cfg)
	require.NoError(t, err)
	require.Nil(t, cfg.DB)
}

type namesNilCollectionSection struct {
	Hosts  []string          `yaml:"hosts" env:"NAMES_NIL_HOSTS"`
	Labels map[string]string `yaml:"labels" env:"NAMES_NIL_LABELS"`
}

type namesNilCollectionConfig struct {
	Section *namesNilCollectionSection `yaml:"section" default:"-"`
}

// TestLoad_EnvTaggedCollectionInvalidPath checks that a variable addressing
// no entry of a tagged collection leaves the nil section holding it nil.
func TestLoad_EnvTaggedCollectionInvalidPath(t *testing.T) {
	t.Setenv("ENVNAME_NAMES_NIL_HOSTS__TYPO", "value")
	t.Setenv("ENVNAME_NAMES_NIL_LABELS__a__b", "value")

	cfg := &namesNilCollectionConfig{}
	_, err := loader.New(nil, loader.WithEnvPrefix("ENVNAME")).Load(cfg)
	require.NoError(t, err)
	require.Nil(t, cfg.Section)
}

type namesBracketConfig struct {
	Hosts  []string          `yaml:"hosts"`
	Labels map[string]string `yaml:"labels"`
}

// TestLoad_EnvBracketsWithCustomDelimiter checks that NAME[key] is read as
// NAME, the configured delimiter, key.
func TestLoad_EnvBracketsWithCustomDelimiter(t *testing.T) {
	t.Setenv("ENVNAME_HOSTS[0]", "h0")
	t.Setenv("ENVNAME_HOSTS_1", "h1")
	t.Setenv("ENVNAME_LABELS[team]", "core")

	cfg := &namesBracketConfig{}
	_, err := loader.New(nil, loader.WithEnvPrefix("ENVNAME"), loader.WithEnvSectionDelimiter("_")).Load(cfg)
	require.NoError(t, err)
	require.Equal(t, []string{"h0", "h1"}, cfg.Hosts)
	require.Equal(t, map[string]string{"team": "core"}, cfg.Labels)
}
