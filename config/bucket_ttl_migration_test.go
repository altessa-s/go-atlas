// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	idempotencyconfig "github.com/altessa-s/go-atlas/config/idempotency"
	lockconfig "github.com/altessa-s/go-atlas/config/lock"
	sagaconfig "github.com/altessa-s/go-atlas/config/saga"
)

// bucketTTLMigrationConfig gathers every config section that exposes the NATS
// bucket TTL migration flag.
type bucketTTLMigrationConfig struct {
	DistributionLock lockconfig.DistributionLock `yaml:"distributionLock"`
	LeaderElector    lockconfig.LeaderElector    `yaml:"leaderElector"`
	Idempotency      idempotencyconfig.Config    `yaml:"idempotency"`
	Saga             sagaconfig.Config           `yaml:"saga"`
}

// TestMigrateBucketTTL_LoadsFromYAML pins the YAML keys of the NATS bucket
// TTL migration flag (their casing follows each section) and that the flag
// stays off unless set.
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
      migrateBucketTTL: true
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

			cfg := loadYAML[bucketTTLMigrationConfig](t, tc.yaml)

			require.Equal(t, tc.want, cfg.DistributionLock.NATS.MigrateBucketTTL, "distributionLock.nats.migrateBucketTTL")
			require.Equal(t, tc.want, cfg.LeaderElector.MigrateBucketTTL, "leaderElector.migrateBucketTTL")
			require.Equal(t, tc.want, cfg.Idempotency.Storage.NATS.MigrateBucketTTL, "idempotency.storage.nats.migrateBucketTTL")
			require.Equal(t, tc.want, cfg.Saga.Storage.NATS.MigrateBucketTTL, "saga.storage.nats.migrate_bucket_ttl")
		})
	}
}
