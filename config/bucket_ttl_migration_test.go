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

// bucketTTLMigrationConfig gathers every config section that exposes the NATS
// bucket-TTL migration flag.
type bucketTTLMigrationConfig struct {
	DistributionLock config.DistributionLock `yaml:"distributionLock"`
	LeaderElector    config.LeaderElector    `yaml:"leaderElector"`
	Idempotency      config.Idempotency      `yaml:"idempotency"`
	Saga             config.Saga             `yaml:"saga"`
}

// TestMigrateBucketTTL_LoadsFromYAML pins the YAML keys of the NATS
// bucket-TTL migration flag (their casing follows each section) and that the
// flag stays off unless set.
func TestMigrateBucketTTL_LoadsFromYAML(t *testing.T) {
	t.Parallel()

	const migrateYAML = `
distributionLock:
  provider: nats
  nats:
    migrateBucketTTL: true
leaderElector:
  provider: nats
  migrateBucketTTL: true
idempotency:
  storage:
    type: nats
    nats:
      migrateBucketTTL: true
saga:
  storage:
    type: nats
    nats:
      migrate_bucket_ttl: true
`
	const defaultYAML = `
distributionLock:
  provider: nats
  nats: {}
leaderElector:
  provider: nats
idempotency:
  storage:
    type: nats
    nats: {}
saga:
  storage:
    type: nats
    nats: {}
`

	cases := []struct {
		name string
		yaml string
		want bool
	}{
		{name: "set", yaml: migrateYAML, want: true},
		{name: "default", yaml: defaultYAML, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "config.yaml")
			require.NoError(t, os.WriteFile(path, []byte(tc.yaml), 0o600))

			cfg := &bucketTTLMigrationConfig{}
			_, err := loader.New(nil, loader.WithPath(path), loader.WithSkipEnv()).Load(cfg)
			require.NoError(t, err)

			require.Equal(t, tc.want, cfg.DistributionLock.Nats.MigrateBucketTTL, "distributionLock.nats.migrateBucketTTL")
			require.Equal(t, tc.want, cfg.LeaderElector.MigrateBucketTTL, "leaderElector.migrateBucketTTL")
			require.Equal(t, tc.want, cfg.Idempotency.Storage.Nats.MigrateBucketTTL, "idempotency.storage.nats.migrateBucketTTL")
			require.Equal(t, tc.want, cfg.Saga.Storage.Nats.MigrateBucketTTL, "saga.storage.nats.migrate_bucket_ttl")
		})
	}
}
