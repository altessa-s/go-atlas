// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpcclientconfig_test

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-nomos/loader"
	"github.com/altessa-s/go-nomos/loader/backend/yaml3"

	grpcclientconfig "github.com/altessa-s/go-atlas/config/grpcclient"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

func TestDefault(t *testing.T) {
	t.Parallel()

	c := grpcclientconfig.Default()
	require.NoError(t, c.Validate())
	require.False(t, c.Resolver.Builtin)
	require.Equal(t, 30*time.Second, c.Resolver.RefreshInterval)
	require.Equal(t, 2*time.Second, c.Resolver.MinInterval)
	require.Equal(t, 10*time.Second, c.Resolver.LookupTimeout)
	require.Equal(t, grpcclientconfig.ResolverBackoff{BaseDelay: time.Second, Multiplier: 1.6, MaxDelay: 5 * time.Second}, c.Resolver.Backoff)
	require.Equal(t, grpcclientconfig.ConnectBackoff{BaseDelay: time.Second, Multiplier: 1.6, MaxDelay: 2 * time.Minute}, c.ConnectBackoff)

	var zero grpcclientconfig.Config
	require.NoError(t, zero.Validate(), "zero fields stand for the defaults")
}

func TestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*grpcclientconfig.Config)
		field  string
	}{
		{"negative refresh interval", func(c *grpcclientconfig.Config) { c.Resolver.RefreshInterval = -time.Second }, "RefreshInterval"},
		{"refresh interval below a second", func(c *grpcclientconfig.Config) { c.Resolver.RefreshInterval = 500 * time.Millisecond }, "RefreshInterval"},
		{"refresh jitter above the maximum", func(c *grpcclientconfig.Config) { c.Resolver.RefreshJitter = new(0.6) }, "RefreshJitter"},
		{"negative refresh jitter", func(c *grpcclientconfig.Config) { c.Resolver.RefreshJitter = new(-0.1) }, "RefreshJitter"},
		{"negative min interval", func(c *grpcclientconfig.Config) { c.Resolver.MinInterval = -time.Second }, "MinInterval"},
		{"min interval above refresh interval", func(c *grpcclientconfig.Config) {
			c.Resolver.RefreshInterval, c.Resolver.MinInterval = 5*time.Second, 6*time.Second
		}, "MinInterval"},
		{"min interval above default refresh interval", func(c *grpcclientconfig.Config) {
			c.Resolver.RefreshInterval, c.Resolver.MinInterval = 0, time.Minute
		}, "MinInterval"},
		{"negative lookup timeout", func(c *grpcclientconfig.Config) { c.Resolver.LookupTimeout = -time.Second }, "LookupTimeout"},
		{"negative resolver base delay", func(c *grpcclientconfig.Config) { c.Resolver.Backoff.BaseDelay = -time.Second }, "Backoff"},
		{"resolver base delay above cap", func(c *grpcclientconfig.Config) { c.Resolver.Backoff.BaseDelay = 6 * time.Second }, "Backoff"},
		{"resolver multiplier below one", func(c *grpcclientconfig.Config) { c.Resolver.Backoff.Multiplier = 0.5 }, "Backoff"},
		{"resolver jitter above one", func(c *grpcclientconfig.Config) { c.Resolver.Backoff.Jitter = new(1.5) }, "Backoff"},
		{"negative resolver jitter", func(c *grpcclientconfig.Config) { c.Resolver.Backoff.Jitter = new(-0.1) }, "Backoff"},
		{"negative resolver cap", func(c *grpcclientconfig.Config) { c.Resolver.Backoff.MaxDelay = -time.Second }, "Backoff"},
		{"negative connect base delay", func(c *grpcclientconfig.Config) { c.ConnectBackoff.BaseDelay = -time.Second }, "ConnectBackoff"},
		{"connect base delay above cap", func(c *grpcclientconfig.Config) {
			c.ConnectBackoff.BaseDelay, c.ConnectBackoff.MaxDelay = 10*time.Second, 5*time.Second
		}, "ConnectBackoff"},
		{"connect multiplier below one", func(c *grpcclientconfig.Config) { c.ConnectBackoff.Multiplier = 0.9 }, "ConnectBackoff"},
		{"connect jitter above one", func(c *grpcclientconfig.Config) { c.ConnectBackoff.Jitter = new(2.0) }, "ConnectBackoff"},
		{"negative connect cap", func(c *grpcclientconfig.Config) { c.ConnectBackoff.MaxDelay = -time.Second }, "ConnectBackoff"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := grpcclientconfig.Default()
			tc.mutate(&c)
			err := c.Validate()
			require.Error(t, err)
			errs, ok := errors.AsType[validation.Errors](err)
			require.True(t, ok, "want validation.Errors, got %T", err)
			section := "Resolver"
			if tc.field == "ConnectBackoff" {
				section = "ConnectBackoff"
			}
			require.Equal(t, []string{section}, slices.Collect(maps.Keys(errs)))
			if section == "Resolver" {
				nested, ok := errors.AsType[validation.Errors](errs[section])
				require.True(t, ok)
				require.Contains(t, nested, tc.field)
			}
		})
	}

	accepted := grpcclientconfig.Default()
	accepted.Resolver.Backoff.Jitter = new(0.0)
	accepted.Resolver.RefreshJitter = new(0.5)
	accepted.ConnectBackoff.Jitter = new(1.0)
	accepted.Resolver.RefreshInterval, accepted.Resolver.MinInterval = time.Second, time.Second
	require.NoError(t, accepted.Validate(), "jitters at their bounds and a min interval equal to the refresh interval are valid")
}

type root struct {
	GRPCClient grpcclientconfig.Config `yaml:"grpcClient"`
}

// An omitted section and omitted keys load as the defaults; an explicit zero
// jitter is kept, an omitted one stays unset.
func TestLoadYAML(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.yaml")
	data := "grpcClient:\n  resolver:\n    refreshInterval: 10s\n    refreshJitter: 0\n    backoff:\n      jitter: 0\n  connectBackoff:\n    maxDelay: 15s\n"
	require.NoError(t, os.WriteFile(path, []byte(data), 0o600))
	var c root
	_, err := loader.New(&yaml3.Backend{}, loader.WithPath(path), loader.WithSkipEnv()).Load(&c)
	require.NoError(t, err)
	require.NoError(t, c.GRPCClient.Validate())

	want := grpcclientconfig.Default()
	want.Resolver.RefreshInterval = 10 * time.Second
	want.Resolver.RefreshJitter = new(0.0)
	want.Resolver.Backoff.Jitter = new(0.0)
	want.ConnectBackoff.MaxDelay = 15 * time.Second
	require.Equal(t, want, c.GRPCClient)

	var omitted root
	_, err = loader.New(nil, loader.WithSkipEnv()).Load(&omitted)
	require.NoError(t, err)
	require.Equal(t, grpcclientconfig.Default(), omitted.GRPCClient)
}

// TestLoadEnv is serial: it sets process environment variables.
func TestLoadEnv(t *testing.T) {
	t.Setenv("GRPCCLIENTTEST_GRPC_CLIENT__RESOLVER__BUILTIN", "true")
	t.Setenv("GRPCCLIENTTEST_GRPC_CLIENT__RESOLVER__BACKOFF__MAX_DELAY", "3s")
	t.Setenv("GRPCCLIENTTEST_GRPC_CLIENT__CONNECT_BACKOFF__JITTER", "0.5")
	var c root
	_, err := loader.New(nil, loader.WithEnvPrefix("GRPCCLIENTTEST")).Load(&c)
	require.NoError(t, err)
	require.NoError(t, c.GRPCClient.Validate())
	require.True(t, c.GRPCClient.Resolver.Builtin)
	require.Equal(t, 3*time.Second, c.GRPCClient.Resolver.Backoff.MaxDelay)
	require.Equal(t, new(0.5), c.GRPCClient.ConnectBackoff.Jitter)
	require.Equal(t, 30*time.Second, c.GRPCClient.Resolver.RefreshInterval)
}
