// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package loader_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config/loader"
)

// All tests in this file mutate process environment via t.Setenv, which is
// incompatible with t.Parallel — keep them serial.

type spellItem struct {
	MaxConns int `yaml:"maxconns"`
}

type spellSection struct {
	MaxConns    int      `yaml:"maxconns"`
	Hosts       []string `yaml:"hosts"`
	BackupHosts []string `yaml:"backups"`
}

type spellConfig struct {
	MaxConns int                  `yaml:"maxconns"`
	MongoDB  spellSection         `yaml:"mongodb"`
	Store    *spellSection        `yaml:"filestore"`
	Items    []spellItem          `yaml:"items"`
	Groups   map[string]spellItem `yaml:"groups"`
}

// TestLoad_EnvBothSpellingsTagWins checks that when a variable is set in both
// the struct tag and the Go field name spelling, the struct tag spelling wins
// on every path, every time.
func TestLoad_EnvBothSpellingsTagWins(t *testing.T) {
	pairs := [][2]string{
		{"MAXCONNS", "MAX_CONNS"},
		{"MONGODB__MAXCONNS", "MONGO_DB__MAX_CONNS"},
		{"MONGODB__HOSTS__0", "MONGO_DB__HOSTS__0"},
		{"FILESTORE__MAXCONNS", "STORE__MAX_CONNS"},
		{"FILESTORE__HOSTS__0", "STORE__HOSTS__0"},
		{"ITEMS__0__MAXCONNS", "ITEMS__0__MAX_CONNS"},
		{"GROUPS__east__MAXCONNS", "GROUPS__east__MAX_CONNS"},
	}
	for _, p := range pairs {
		t.Setenv("ENVSPELL_"+p[0], "1")
		t.Setenv("ENVSPELL_"+p[1], "2")
	}

	for range 50 {
		cfg := &spellConfig{}
		_, err := loader.New(nil, loader.WithEnvPrefix("ENVSPELL")).Load(cfg)
		require.NoError(t, err)
		require.Equal(t, 1, cfg.MaxConns)
		require.Equal(t, 1, cfg.MongoDB.MaxConns)
		require.Equal(t, []string{"1"}, cfg.MongoDB.Hosts)
		require.NotNil(t, cfg.Store)
		require.Equal(t, 1, cfg.Store.MaxConns)
		require.Equal(t, []string{"1"}, cfg.Store.Hosts)
		require.Equal(t, []spellItem{{MaxConns: 1}}, cfg.Items)
		require.Equal(t, map[string]spellItem{"east": {MaxConns: 1}}, cfg.Groups)
	}
}

// TestLoad_EnvMixedSpellingsNameOrder checks that of two mixed spellings
// with as many struct tag levels, the first in name order wins on every path.
func TestLoad_EnvMixedSpellingsNameOrder(t *testing.T) {
	t.Setenv("ENVSPELL_FILESTORE__MAX_CONNS", "3")
	t.Setenv("ENVSPELL_STORE__MAXCONNS", "4")
	t.Setenv("ENVSPELL_FILESTORE__BACKUP_HOSTS__0", "3")
	t.Setenv("ENVSPELL_STORE__BACKUPS__0", "4")

	for range 50 {
		cfg := &spellConfig{}
		_, err := loader.New(nil, loader.WithEnvPrefix("ENVSPELL")).Load(cfg)
		require.NoError(t, err)
		require.NotNil(t, cfg.Store)
		require.Equal(t, 3, cfg.Store.MaxConns)
		require.Equal(t, []string{"3"}, cfg.Store.BackupHosts)
	}
}

type placeholderConfig struct {
	AppName string `yaml:"appName" env:"APP_NAME"`
	Bare    string `yaml:"bare" env:"APP_"`
	Skipped string `yaml:"skipped" env:"-"`
}

// TestLoad_EnvTagPlaceholder checks the APP_ placeholder of env tags: it is
// replaced by the configured prefix, a tag that is only the placeholder names
// itself, and env:"-" keeps the environment out of the field.
func TestLoad_EnvTagPlaceholder(t *testing.T) {
	t.Setenv("ENVTAG_NAME", "name")
	t.Setenv("ENVTAG_APP_", "bare")
	t.Setenv("ENVTAG_BARE", "leaked")
	t.Setenv("ENVTAG_-", "leaked")
	t.Setenv("ENVTAG_SKIPPED", "leaked")

	cfg := &placeholderConfig{}
	_, err := loader.New(nil, loader.WithEnvPrefix("ENVTAG")).Load(cfg)
	require.NoError(t, err)
	require.Equal(t, "name", cfg.AppName)
	require.Equal(t, "bare", cfg.Bare)
	require.Empty(t, cfg.Skipped)
}

type secretValueSection struct {
	Port int `yaml:"port"`
}

type secretValueConfig struct {
	Port    int                  `yaml:"port"`
	Timeout time.Duration        `yaml:"timeout"`
	Enabled bool                 `yaml:"enabled"`
	Ports   []int                `yaml:"ports"`
	Limits  map[string]int       `yaml:"limits"`
	Section secretValueSection   `yaml:"section"`
	Workers []secretValueSection `yaml:"workers"`
}

// TestLoad_EnvErrorOmitsValue checks that a value that fails to parse is not
// quoted in the error, which names the variable instead: the value may be an
// expanded secret.
func TestLoad_EnvErrorOmitsValue(t *testing.T) {
	const secret = "s3cr3t-Pa55w0rd"
	tests := []struct {
		name string
		env  string
	}{
		{name: "int", env: "ENVSECRET_PORT"},
		{name: "duration", env: "ENVSECRET_TIMEOUT"},
		{name: "bool", env: "ENVSECRET_ENABLED"},
		{name: "slice", env: "ENVSECRET_PORTS"},
		{name: "slice element", env: "ENVSECRET_PORTS__0"},
		{name: "map entry", env: "ENVSECRET_LIMITS__k"},
		{name: "map", env: "ENVSECRET_LIMITS"},
		{name: "section field", env: "ENVSECRET_SECTION__PORT"},
		{name: "struct element", env: "ENVSECRET_WORKERS__0__PORT"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.env, secret)

			_, err := loader.New(nil, loader.WithEnvPrefix("ENVSECRET")).Load(&secretValueConfig{})
			require.ErrorIs(t, err, loader.ErrBindEnv)
			require.NotContains(t, err.Error(), secret)
			require.NotContains(t, err.Error(), "Pa55")
			require.Contains(t, err.Error(), tt.env)
		})
	}
}

type spellRetry struct {
	MaxConns int `yaml:"maxconns"`
}

type spellIndexedItem struct {
	RetryPolicy spellRetry `yaml:"retry"`
}

type spellIndexedSection struct {
	Items []spellIndexedItem `yaml:"items"`
}

type spellIndexedConfig struct {
	MongoDB spellIndexedSection `yaml:"mongodb"`
}

// TestLoad_EnvSpellingRankAcrossIndex checks that spellings are ranked by
// all levels of a variable, on both sides of a slice index.
func TestLoad_EnvSpellingRankAcrossIndex(t *testing.T) {
	// Two Go field name levels after the index, against one before it.
	t.Setenv("ENVSPELL_MONGODB__ITEMS__0__RETRY_POLICY__MAX_CONNS", "2")
	t.Setenv("ENVSPELL_MONGO_DB__ITEMS__0__RETRY__MAXCONNS", "1")

	for range 50 {
		cfg := &spellIndexedConfig{}
		_, err := loader.New(nil, loader.WithEnvPrefix("ENVSPELL")).Load(cfg)
		require.NoError(t, err)
		require.Equal(t, []spellIndexedItem{{RetryPolicy: spellRetry{MaxConns: 1}}}, cfg.MongoDB.Items)
	}
}

type spellZeroConfig struct {
	Hosts []string    `yaml:"hosts"`
	Items []spellItem `yaml:"items"`
}

// TestLoad_EnvEquivalentIndexes checks that indexes spelled differently but
// equal address one element, and the first variable in name order wins:
// HOSTS__0 before HOSTS__00, but ITEMS__00__MAXCONNS before
// ITEMS__0__MAXCONNS, as "0" sorts before "_".
func TestLoad_EnvEquivalentIndexes(t *testing.T) {
	t.Setenv("ENVSPELL_HOSTS__0", "a")
	t.Setenv("ENVSPELL_HOSTS__00", "b")
	t.Setenv("ENVSPELL_ITEMS__0__MAXCONNS", "2")
	t.Setenv("ENVSPELL_ITEMS__00__MAXCONNS", "1")

	for range 50 {
		cfg := &spellZeroConfig{}
		_, err := loader.New(nil, loader.WithEnvPrefix("ENVSPELL")).Load(cfg)
		require.NoError(t, err)
		require.Equal(t, []string{"a"}, cfg.Hosts)
		require.Equal(t, []spellItem{{MaxConns: 1}}, cfg.Items)
	}
}

type spellOwnedConfig struct {
	Hosts  []string `yaml:"hosts" env:"LIST"`
	Single string   `yaml:"single" env:"LIST__0"`
}

// TestLoad_EnvWholeFieldOwnsVariable checks that a variable the tag of a
// field names whole is not also read as an entry of a shorter tag.
func TestLoad_EnvWholeFieldOwnsVariable(t *testing.T) {
	t.Setenv("ENVSPELL_LIST__0", "x")

	cfg := &spellOwnedConfig{}
	_, err := loader.New(nil, loader.WithEnvPrefix("ENVSPELL")).Load(cfg)
	require.NoError(t, err)
	require.Equal(t, "x", cfg.Single)
	require.Empty(t, cfg.Hosts)
}
