// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package schedulerconfig_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config/loader"

	schedulerconfig "github.com/altessa-s/go-atlas/config/scheduler"
)

// HistoryTTL used to carry `default:"-"`, which the loader parses as a
// duration and rejects, so any config with a storage.redis section failed to
// load. Zero now means "no expiry".
func TestSchedulerStorageRedisConfig_HistoryTTLLoads(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		yaml      string
		wantRedis bool
		wantTTL   time.Duration
	}{
		{
			name:      "RedisSectionOmitted",
			yaml:      "scheduler:\n  storage:\n    type: memory\n",
			wantRedis: false,
		},
		{
			name:      "HistoryTTLOmitted",
			yaml:      "scheduler:\n  storage:\n    type: redis\n    redis:\n      keyPrefix: jobs\n",
			wantRedis: true,
			wantTTL:   0,
		},
		{
			name:      "HistoryTTLZero",
			yaml:      "scheduler:\n  storage:\n    type: redis\n    redis:\n      historyTtl: \"0s\"\n",
			wantRedis: true,
			wantTTL:   0,
		},
		{
			name:      "HistoryTTLPositive",
			yaml:      "scheduler:\n  storage:\n    type: redis\n    redis:\n      historyTtl: \"1h\"\n",
			wantRedis: true,
			wantTTL:   time.Hour,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "config.yaml")
			require.NoError(t, os.WriteFile(path, []byte(tc.yaml), 0o600))

			type wrapper struct {
				Scheduler schedulerconfig.Config `yaml:"scheduler"`
			}
			cfg := &wrapper{}
			_, err := loader.New(nil, loader.WithPath(path), loader.WithSkipEnv()).Load(cfg)
			require.NoError(t, err)

			require.NotNil(t, cfg.Scheduler.Storage)
			if !tc.wantRedis {
				require.Nil(t, cfg.Scheduler.Storage.Redis)
				return
			}
			require.NotNil(t, cfg.Scheduler.Storage.Redis)
			require.Equal(t, tc.wantTTL, cfg.Scheduler.Storage.Redis.HistoryTTL)
		})
	}
}

func TestSchedulerStorageSQLConfig(t *testing.T) {
	t.Parallel()

	t.Run("DefaultsLoad", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "config.yaml")
		require.NoError(t, os.WriteFile(path, []byte("scheduler:\n  storage:\n    type: sql\n    sql:\n      dialect: mysql\n"), 0o600))

		type wrapper struct {
			Scheduler schedulerconfig.Config `yaml:"scheduler"`
		}
		cfg := &wrapper{}
		_, err := loader.New(nil, loader.WithPath(path), loader.WithSkipEnv()).Load(cfg)
		require.NoError(t, err)
		require.NotNil(t, cfg.Scheduler.Storage.SQL)
		require.Equal(t, schedulerconfig.SQLDialectMySQL, cfg.Scheduler.Storage.SQL.Dialect)
		require.Equal(t, "scheduler_tasks", cfg.Scheduler.Storage.SQL.TasksTable)
		require.Equal(t, "scheduler_history", cfg.Scheduler.Storage.SQL.HistoryTable)
		require.NoError(t, cfg.Scheduler.Storage.Validate())
	})

	t.Run("Validation", func(t *testing.T) {
		t.Parallel()
		valid := schedulerconfig.DefaultStorageSQLConfig()
		storage := schedulerconfig.StorageConfig{Type: schedulerconfig.StorageTypeSQL, SQL: &valid}
		require.NoError(t, storage.Validate())

		bad := valid
		bad.Dialect = "oracle"
		require.Error(t, (&schedulerconfig.StorageConfig{Type: schedulerconfig.StorageTypeSQL, SQL: &bad}).Validate())

		noTable := valid
		noTable.TasksTable = ""
		require.Error(t, (&schedulerconfig.StorageConfig{Type: schedulerconfig.StorageTypeSQL, SQL: &noTable}).Validate())
	})
}

func TestScheduler_InstanceID(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		yaml    string
		want    string
		wantErr bool
	}{
		{name: "Omitted", yaml: "scheduler:\n  tickInterval: 1s\n"},
		{name: "Set", yaml: "scheduler:\n  instanceId: pod-0\n", want: "pod-0"},
		{
			name:    "TooLong",
			yaml:    "scheduler:\n  instanceId: " + strings.Repeat("x", schedulerconfig.MaxSchedulerInstanceIDLength+1) + "\n",
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "config.yaml")
			require.NoError(t, os.WriteFile(path, []byte(tc.yaml), 0o600))

			type wrapper struct {
				Scheduler schedulerconfig.Config `yaml:"scheduler"`
			}
			cfg := &wrapper{}
			_, err := loader.New(nil, loader.WithPath(path), loader.WithSkipEnv()).Load(cfg)
			require.NoError(t, err)

			if tc.wantErr {
				require.Error(t, cfg.Scheduler.Validate())
				return
			}
			require.Equal(t, tc.want, cfg.Scheduler.InstanceID)
			require.NoError(t, cfg.Scheduler.Validate())
		})
	}
}
