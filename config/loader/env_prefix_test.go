// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package loader_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config/loader"
	"github.com/altessa-s/go-atlas/core/runtime/appinfo"
)

// All tests in this file mutate process environment via t.Setenv, which is
// incompatible with t.Parallel — keep them serial.

type prefixDatabase struct {
	Host string `yaml:"host"`
}

type prefixConfig struct {
	Port     int            `yaml:"port"`
	Database prefixDatabase `yaml:"database"`
	Hosts    []string       `yaml:"hosts"`
}

// TestLoad_EnvPrefixSeparatorNormalized checks that a prefix with or without
// its trailing "_" names the same variables: WithEnvPrefix("APP") and
// WithEnvPrefix("APP_") both read APP_PORT, APP_DATABASE__HOST and
// APP_HOSTS__0.
func TestLoad_EnvPrefixSeparatorNormalized(t *testing.T) {
	t.Setenv("ENVPFX_PORT", "9090")
	t.Setenv("ENVPFX_DATABASE__HOST", "db.internal")
	t.Setenv("ENVPFX_HOSTS__0", "a")

	for _, prefix := range []string{"ENVPFX", "ENVPFX_", "ENVPFX__", " ENVPFX "} {
		t.Run(prefix, func(t *testing.T) {
			cfg := &prefixConfig{}
			_, err := loader.New(nil, loader.WithEnvPrefix(prefix)).Load(cfg)
			require.NoError(t, err)
			require.Equal(t, 9090, cfg.Port)
			require.Equal(t, "db.internal", cfg.Database.Host)
			require.Equal(t, []string{"a"}, cfg.Hosts)
		})
	}
}

// TestLoad_EnvPrefixWithoutSeparatorIgnored checks that a variable glued to
// the prefix without the "_" separator is not read.
func TestLoad_EnvPrefixWithoutSeparatorIgnored(t *testing.T) {
	t.Setenv("ENVGLUEPORT", "9090")
	t.Setenv("ENVGLUEDATABASE__HOST", "db.internal")

	cfg := &prefixConfig{}
	_, err := loader.New(nil, loader.WithEnvPrefix("ENVGLUE")).Load(cfg)
	require.NoError(t, err)
	require.Zero(t, cfg.Port)
	require.Empty(t, cfg.Database.Host)
}

type prefixLeakConfig struct {
	Port     int               `yaml:"port"`
	Database prefixDatabase    `yaml:"database"`
	Hosts    []string          `yaml:"hosts"`
	Labels   map[string]string `yaml:"labels"`
}

// TestLoad_EnvPrefixRequired checks that with a prefix configured, variables
// without it never override the configuration, on any path: top-level
// fields, nested sections, indexed slices and map entries.
func TestLoad_EnvPrefixRequired(t *testing.T) {
	t.Setenv("PORT", "1")
	t.Setenv("DATABASE__HOST", "leaked")
	t.Setenv("HOSTS__0", "leaked")
	t.Setenv("LABELS__k", "leaked")
	t.Setenv("ENVREQ_DATABASE__HOST", "db.internal")

	cfg := &prefixLeakConfig{}
	_, err := loader.New(nil, loader.WithEnvPrefix("ENVREQ")).Load(cfg)
	require.NoError(t, err)
	require.Zero(t, cfg.Port)
	require.Equal(t, "db.internal", cfg.Database.Host)
	require.Empty(t, cfg.Hosts)
	require.Empty(t, cfg.Labels)
}

// TestLoad_EnvPrefixRequiredStrict checks that strict mode does not reject a
// configuration because of an unrelated variable without the prefix.
func TestLoad_EnvPrefixRequiredStrict(t *testing.T) {
	t.Setenv("DATABASE__NONEXISTENT", "value")

	cfg := &prefixLeakConfig{}
	_, err := loader.New(nil, loader.WithEnvPrefix("ENVREQ"), loader.WithStrict()).Load(cfg)
	require.NoError(t, err)
}

// TestLoad_NoEnvPrefixReadsAllNames checks that without a prefix every
// variable is considered, as before.
func TestLoad_NoEnvPrefixReadsAllNames(t *testing.T) {
	t.Setenv("PORT", "1")
	t.Setenv("DATABASE__HOST", "db.internal")
	t.Setenv("HOSTS__0", "a")
	t.Setenv("LABELS__k", "v")

	cfg := &prefixLeakConfig{}
	_, err := loader.New(nil).Load(cfg)
	require.NoError(t, err)
	require.Equal(t, 1, cfg.Port)
	require.Equal(t, "db.internal", cfg.Database.Host)
	require.Equal(t, []string{"a"}, cfg.Hosts)
	require.Equal(t, map[string]string{"k": "v"}, cfg.Labels)
}

type prefixTagConfig struct {
	Name string `yaml:"name" env:"APP_NAME"`
	Host string `yaml:"host" env:"DB_HOST"`
	Mode string `yaml:"mode" env:"ENVTAG_MODE"`
}

// TestLoad_EnvTagUnderPrefix checks how an env tag maps under a prefix: a
// plain tag and a tag that already carries the prefix are read with it, an
// "APP_" tag has that part replaced by it, and the bare tag is ignored.
func TestLoad_EnvTagUnderPrefix(t *testing.T) {
	t.Setenv("ENVTAG_NAME", "svc")
	t.Setenv("ENVTAG_DB_HOST", "db.internal")
	t.Setenv("ENVTAG_MODE", "fast")
	t.Setenv("DB_HOST", "leaked")

	cfg := &prefixTagConfig{}
	_, err := loader.New(nil, loader.WithEnvPrefix("ENVTAG")).Load(cfg)
	require.NoError(t, err)
	require.Equal(t, "svc", cfg.Name)
	require.Equal(t, "db.internal", cfg.Host)
	require.Equal(t, "fast", cfg.Mode)
}

// TestLoad_EnvPrefixMatchesAppinfo checks that the loader normalizes a prefix
// exactly as appinfo.NormalizeEnvKeyPrefix does: a raw prefix and its
// normalized form read the same variables.
func TestLoad_EnvPrefixMatchesAppinfo(t *testing.T) {
	t.Setenv("ENVX_APP_PORT", "9090")

	for _, raw := range []string{"envx-app", " ENVX.APP ", "ENVX_APP__", "envx_app"} {
		t.Run(raw, func(t *testing.T) {
			require.Equal(t, "ENVX_APP_", appinfo.NormalizeEnvKeyPrefix(raw))

			for _, prefix := range []string{raw, appinfo.NormalizeEnvKeyPrefix(raw)} {
				cfg := &prefixConfig{}
				_, err := loader.New(nil, loader.WithEnvPrefix(prefix)).Load(cfg)
				require.NoError(t, err)
				require.Equal(t, 9090, cfg.Port, "prefix %q", prefix)
			}
		})
	}
}

// TestLoad_EnvPrefixWhitespaceOnly checks that a prefix of whitespace and
// separators means no prefix, as in appinfo.
func TestLoad_EnvPrefixWhitespaceOnly(t *testing.T) {
	t.Setenv("PORT", "1")

	for _, raw := range []string{"   ", " _ ", "__"} {
		require.Empty(t, appinfo.NormalizeEnvKeyPrefix(raw))
		cfg := &prefixConfig{}
		_, err := loader.New(nil, loader.WithEnvPrefix(raw)).Load(cfg)
		require.NoError(t, err)
		require.Equal(t, 1, cfg.Port, "prefix %q", raw)
	}
}
