// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config/loader"

	lockconfig "github.com/altessa-s/go-atlas/config/lock"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
)

type kvStorageConfig struct {
	DistributionLock lockconfig.DistributionLock `yaml:"distributionLock"`
	LeaderElector    lockconfig.LeaderElector    `yaml:"leaderElector"`
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
	require.Equal(t, storageconfig.KVStorageFile, cfg.DistributionLock.Nats.Storage)
	require.Equal(t, storageconfig.KVStorageFile, cfg.LeaderElector.Storage)

	cfg = loadYAML[kvStorageConfig](t, "distributionLock:\n  provider: nats\n  nats: {}\nleaderElector:\n  provider: nats\n")
	require.Equal(t, storageconfig.KVStorageMemory, cfg.DistributionLock.Nats.Storage)
	require.Equal(t, storageconfig.KVStorageMemory, cfg.LeaderElector.Storage)
	require.Equal(t, storageconfig.KVStorageMemory, lockconfig.DefaultDistributionLockNats().Storage)
	require.Equal(t, storageconfig.KVStorageMemory, lockconfig.DefaultLeaderElector().Storage)
}

func TestKVStorage_Validate(t *testing.T) {
	t.Parallel()

	for _, storage := range []storageconfig.KVStorageType{"", storageconfig.KVStorageMemory, storageconfig.KVStorageFile} {
		dl := lockconfig.DistributionLock{Provider: lockconfig.DistributionLockProviderNats, Nats: &lockconfig.DistributionLockNats{Storage: storage}}
		require.NoError(t, dl.Validate(), storage)
		le := lockconfig.LeaderElector{Provider: lockconfig.LeaderElectorProviderNats, Ttl: lockconfig.DefaultLeaderElector().Ttl, Storage: storage}
		require.NoError(t, le.Validate(), storage)
	}

	dl := lockconfig.DistributionLock{Provider: lockconfig.DistributionLockProviderNats, Nats: &lockconfig.DistributionLockNats{Storage: "disk"}}
	require.Error(t, dl.Validate())
	le := lockconfig.LeaderElector{Provider: lockconfig.LeaderElectorProviderNats, Ttl: lockconfig.DefaultLeaderElector().Ttl, Storage: "disk"}
	require.Error(t, le.Validate())
}
