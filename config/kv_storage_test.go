// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/config/loader"
)

type kvStorageConfig struct {
	DistributionLock config.DistributionLock `yaml:"distributionLock"`
	LeaderElector    config.LeaderElector    `yaml:"leaderElector"`
}

// loadYAML loads yaml into a fresh T through the config loader, without
// environment overrides.
func loadYAML[T any](t *testing.T, yaml string) *T {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0o600))
	cfg := new(T)
	_, err := loader.New(nil, loader.WithPath(path), loader.WithSkipEnv()).Load(cfg)
	require.NoError(t, err)
	return cfg
}

// TestKVStorage_LoadsFromYAML pins the storage key of the dlock and leader
// election buckets and its memory default.
func TestKVStorage_LoadsFromYAML(t *testing.T) {
	t.Parallel()

	cfg := loadYAML[kvStorageConfig](t, "distributionLock:\n  provider: nats\n  nats:\n    storage: file\nleaderElector:\n  provider: nats\n  storage: file\n")
	require.Equal(t, config.KVStorageFile, cfg.DistributionLock.Nats.Storage)
	require.Equal(t, config.KVStorageFile, cfg.LeaderElector.Storage)

	cfg = loadYAML[kvStorageConfig](t, "distributionLock:\n  provider: nats\n  nats: {}\nleaderElector:\n  provider: nats\n")
	require.Equal(t, config.KVStorageMemory, cfg.DistributionLock.Nats.Storage)
	require.Equal(t, config.KVStorageMemory, cfg.LeaderElector.Storage)
	require.Equal(t, config.KVStorageMemory, config.DefaultDistributionLockNats().Storage)
	require.Equal(t, config.KVStorageMemory, config.DefaultLeaderElector().Storage)
}

func TestKVStorage_Validate(t *testing.T) {
	t.Parallel()

	for _, storage := range []config.KVStorageType{"", config.KVStorageMemory, config.KVStorageFile} {
		dl := config.DistributionLock{Provider: config.DistributionLockProviderNats, Nats: &config.DistributionLockNats{Storage: storage}}
		require.NoError(t, dl.Validate(), storage)
		le := config.LeaderElector{Provider: config.LeaderElectorProviderNats, Ttl: config.DefaultLeaderElector().Ttl, Storage: storage}
		require.NoError(t, le.Validate(), storage)
	}

	dl := config.DistributionLock{Provider: config.DistributionLockProviderNats, Nats: &config.DistributionLockNats{Storage: "disk"}}
	require.Error(t, dl.Validate())
	le := config.LeaderElector{Provider: config.LeaderElectorProviderNats, Ttl: config.DefaultLeaderElector().Ttl, Storage: "disk"}
	require.Error(t, le.Validate())
}
