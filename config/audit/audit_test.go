// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auditconfig_test

import (
	"errors"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-nomos/loader"

	"github.com/altessa-s/go-atlas-core/types/redacted"

	auditconfig "github.com/altessa-s/go-atlas/config/audit"
	dispatchconfig "github.com/altessa-s/go-atlas/config/dispatch"
	pagingconfig "github.com/altessa-s/go-atlas/config/paging"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

func key(n int) redacted.RedactedString {
	return redacted.RedactedString(strings.Repeat("k", n))
}

func validConfig() auditconfig.Config {
	return auditconfig.Config{
		Enabled: true,
		Storage: auditconfig.Storage{
			Type: auditconfig.StorageTypeClickHouse,
			ClickHouse: &auditconfig.StorageClickHouse{
				TableName:       "audit_events",
				Engine:          "ReplacingMergeTree",
				MaxBatchSize:    10_000,
				DDLTimeout:      30 * time.Second,
				TimeRangeMode:   "warn",
				SchemaCheckMode: "warn",
			},
		},
		Paging: &pagingconfig.Config{SigningKey: key(32), TokenTTL: 24 * time.Hour},
	}
}

func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(c *auditconfig.Config)
		wantErr bool
	}{
		{"valid", func(*auditconfig.Config) {}, false},
		{"unknown storage type", func(c *auditconfig.Config) { c.Storage.Type = "sqlite" }, true},
		{"omitted storage uses default memory", func(c *auditconfig.Config) {
			c.Storage = auditconfig.Storage{}
		}, false},
		{"omitted storage type with memory block", func(c *auditconfig.Config) {
			c.Storage = auditconfig.Storage{Memory: &auditconfig.StorageMemory{MaxEvents: 500}}
		}, false},
		{"omitted storage type with zero maxEvents", func(c *auditconfig.Config) {
			c.Storage = auditconfig.Storage{Memory: &auditconfig.StorageMemory{MaxEvents: 0}}
		}, true},
		{"memory needs no block", func(c *auditconfig.Config) {
			c.Storage = auditconfig.Storage{Type: auditconfig.StorageTypeMemory}
		}, false},
		{"memory with valid maxEvents", func(c *auditconfig.Config) {
			c.Storage = auditconfig.Storage{
				Type:   auditconfig.StorageTypeMemory,
				Memory: &auditconfig.StorageMemory{MaxEvents: 1000},
			}
		}, false},
		{"memory with zero maxEvents", func(c *auditconfig.Config) {
			c.Storage = auditconfig.Storage{
				Type:   auditconfig.StorageTypeMemory,
				Memory: &auditconfig.StorageMemory{MaxEvents: 0},
			}
		}, true},
		{"memory with negative maxEvents", func(c *auditconfig.Config) {
			c.Storage = auditconfig.Storage{
				Type:   auditconfig.StorageTypeMemory,
				Memory: &auditconfig.StorageMemory{MaxEvents: -1},
			}
		}, true},
		{"mongo ignores non-positive memory block", func(c *auditconfig.Config) {
			c.Storage = auditconfig.Storage{
				Type:   auditconfig.StorageTypeMongo,
				Mongo:  &auditconfig.StorageMongo{},
				Memory: &auditconfig.StorageMemory{MaxEvents: 0},
			}
		}, false},
		{"clickhouse ignores non-positive memory block", func(c *auditconfig.Config) {
			c.Storage.Memory = &auditconfig.StorageMemory{MaxEvents: -1}
		}, false},
		{"mongo requires its block", func(c *auditconfig.Config) {
			c.Storage = auditconfig.Storage{Type: auditconfig.StorageTypeMongo}
		}, true},
		{"mongo with its block", func(c *auditconfig.Config) {
			c.Storage = auditconfig.Storage{Type: auditconfig.StorageTypeMongo, Mongo: &auditconfig.StorageMongo{}}
		}, false},
		{"clickhouse requires its block", func(c *auditconfig.Config) { c.Storage.ClickHouse = nil }, true},
		{"clickhouse without table", func(c *auditconfig.Config) { c.Storage.ClickHouse.TableName = "" }, true},
		{"clickhouse without engine", func(c *auditconfig.Config) { c.Storage.ClickHouse.Engine = "" }, true},
		{"clickhouse zero batch", func(c *auditconfig.Config) { c.Storage.ClickHouse.MaxBatchSize = 0 }, true},
		{"clickhouse negative ddl timeout", func(c *auditconfig.Config) { c.Storage.ClickHouse.DDLTimeout = -time.Second }, true},
		{"clickhouse unknown time range mode", func(c *auditconfig.Config) { c.Storage.ClickHouse.TimeRangeMode = "strict" }, true},
		{"clickhouse unknown schema check mode", func(c *auditconfig.Config) { c.Storage.ClickHouse.SchemaCheckMode = "on" }, true},
		{"clickhouse empty modes use defaults", func(c *auditconfig.Config) {
			ch := c.Storage.ClickHouse
			ch.TimeRangeMode, ch.SchemaCheckMode = "", ""
		}, false},
		{"enabled wal without dir", func(c *auditconfig.Config) {
			c.Dispatch.WAL = &dispatchconfig.WAL{Enabled: true}
		}, true},
		{"enabled wal with dir", func(c *auditconfig.Config) {
			c.Dispatch.WAL = &dispatchconfig.WAL{Enabled: true, Dir: "/var/lib/audit/wal", MaxSegmentBytes: 1, MaxBytes: 1}
		}, false},
		{"no paging", func(c *auditconfig.Config) { c.Paging = nil }, false},
		{"paging without signing key", func(c *auditconfig.Config) { c.Paging.SigningKey = "" }, true},
		{"signing key one byte short", func(c *auditconfig.Config) { c.Paging.SigningKey = key(31) }, true},
		{"previous key at the minimum", func(c *auditconfig.Config) {
			c.Paging.PreviousKeys = []redacted.RedactedString{key(32)}
		}, false},
		{"previous key one byte short", func(c *auditconfig.Config) {
			c.Paging.PreviousKeys = []redacted.RedactedString{key(32), key(31)}
		}, true},
		{"zero token ttl", func(c *auditconfig.Config) { c.Paging.TokenTTL = 0 }, false},
		{"negative token ttl", func(c *auditconfig.Config) { c.Paging.TokenTTL = -time.Second }, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := validConfig()
			tc.mutate(&cfg)
			if tc.wantErr {
				require.Error(t, cfg.Validate())
				return
			}
			require.NoError(t, cfg.Validate())
		})
	}
}

func TestSQLTTLMatchesMongo(t *testing.T) {
	t.Parallel()
	for _, ttl := range []time.Duration{0, time.Hour, -time.Second} {
		t.Run(ttl.String(), func(t *testing.T) {
			t.Parallel()
			mongo := auditconfig.StorageMongo{TTL: ttl}
			sql := auditconfig.StorageSQL{Dialect: auditconfig.SQLDialectPostgres, TableName: "audit_events", TTL: ttl}
			// Mongo has no TTL validation; nonpositive values leave retention disabled.
			mongoCfg := validConfig()
			mongoCfg.Storage = auditconfig.Storage{Type: auditconfig.StorageTypeMongo, Mongo: &mongo}
			sqlCfg := validConfig()
			sqlCfg.Storage = auditconfig.Storage{Type: auditconfig.StorageTypeSQL, SQL: &sql}
			require.NoError(t, mongoCfg.Validate())
			require.NoError(t, sqlCfg.Validate())
		})
	}
}

func TestStorageMongo_ValidateTTL(t *testing.T) {
	t.Parallel()

	maxValid := time.Duration(math.MaxInt32) * time.Second
	tests := []struct {
		name    string
		ttl     time.Duration
		wantErr bool
	}{
		{name: "zero disabled", ttl: 0, wantErr: false},
		{name: "sub-second 720ns rejected", ttl: 720 * time.Nanosecond, wantErr: true},
		{name: "sub-second 999ms rejected", ttl: 999 * time.Millisecond, wantErr: true},
		{name: "one second valid", ttl: time.Second, wantErr: false},
		{name: "max int32 seconds valid", ttl: maxValid, wantErr: false},
		{name: "overflow int32 seconds rejected", ttl: maxValid + time.Second, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := validConfig()
			cfg.Storage = auditconfig.Storage{
				Type:  auditconfig.StorageTypeMongo,
				Mongo: &auditconfig.StorageMongo{TTL: tc.ttl},
			}
			err := cfg.Validate()
			if tc.wantErr {
				require.Error(t, err)
				verrs, ok := errors.AsType[validation.Errors](err)
				require.True(t, ok, "want validation.Errors, got %T: %v", err, err)
				storageErrs, ok := errors.AsType[validation.Errors](verrs["Storage"])
				require.True(t, ok, "want Storage validation.Errors, got %v", verrs)
				mongoErrs, ok := errors.AsType[validation.Errors](storageErrs["Mongo"])
				require.True(t, ok, "want Mongo validation.Errors, got %v", storageErrs)
				require.Equal(t, []string{"TTL"}, slices.Collect(maps.Keys(mongoErrs)))
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestConfig_DefaultStorageFromLoader(t *testing.T) {
	t.Parallel()

	loadAuditYAML := func(t *testing.T, content string) auditconfig.Config {
		t.Helper()
		path := filepath.Join(t.TempDir(), "config.yaml")
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

		var root struct {
			Audit auditconfig.Config `yaml:"audit"`
		}
		_, err := loader.New(nil, loader.WithPath(path), loader.WithSkipEnv()).Load(&root)
		require.NoError(t, err)
		return root.Audit
	}

	t.Run("implicit_memory_leaves_type_empty", func(t *testing.T) {
		t.Parallel()
		cfg := loadAuditYAML(t, "audit:\n  enabled: true\n")
		require.Empty(t, cfg.Storage.Type, "loader must leave omitted storage.type empty so factory/audit can warn on default memory fallback")
		require.NoError(t, cfg.Validate())
	})

	t.Run("explicit_memory_populates_defaults", func(t *testing.T) {
		t.Parallel()
		cfg := loadAuditYAML(t, "audit:\n  enabled: true\n  storage:\n    type: memory\n")
		require.Equal(t, auditconfig.StorageTypeMemory, cfg.Storage.Type)
		require.NotNil(t, cfg.Storage.Memory)
		require.Equal(t, auditconfig.DefaultMemoryMaxEvents, cfg.Storage.Memory.MaxEvents)
		require.NoError(t, cfg.Validate())
	})

	t.Run("explicit_mongo_populates_defaults", func(t *testing.T) {
		t.Parallel()
		cfg := loadAuditYAML(t, "audit:\n  enabled: true\n  storage:\n    type: mongo\n")
		require.Equal(t, auditconfig.StorageTypeMongo, cfg.Storage.Type)
		require.NotNil(t, cfg.Storage.Mongo)
		require.Equal(t, "audit_events", cfg.Storage.Mongo.CollectionName)
		require.Equal(t, 30*time.Second, cfg.Storage.Mongo.IndexTimeout)
		require.Zero(t, cfg.Storage.Mongo.TTL)
		require.NoError(t, cfg.Validate())
	})

	t.Run("explicit_clickhouse_populates_defaults", func(t *testing.T) {
		t.Parallel()
		cfg := loadAuditYAML(t, "audit:\n  enabled: true\n  storage:\n    type: clickhouse\n")
		require.Equal(t, auditconfig.StorageTypeClickHouse, cfg.Storage.Type)
		require.NotNil(t, cfg.Storage.ClickHouse)
		require.Equal(t, "audit_events", cfg.Storage.ClickHouse.TableName)
		require.Equal(t, "ReplacingMergeTree", cfg.Storage.ClickHouse.Engine)
		require.Equal(t, 10_000, cfg.Storage.ClickHouse.MaxBatchSize)
		require.Equal(t, 30*time.Second, cfg.Storage.ClickHouse.DDLTimeout)
		require.Equal(t, "warn", cfg.Storage.ClickHouse.TimeRangeMode)
		require.Equal(t, "warn", cfg.Storage.ClickHouse.SchemaCheckMode)
		require.NoError(t, cfg.Validate())
	})

	t.Run("explicit_sqldb_populates_defaults", func(t *testing.T) {
		t.Parallel()
		cfg := loadAuditYAML(t, "audit:\n  enabled: true\n  storage:\n    type: sqldb\n")
		require.Equal(t, auditconfig.StorageTypeSQL, cfg.Storage.Type)
		require.NotNil(t, cfg.Storage.SQL)
		require.Equal(t, auditconfig.SQLDialectPostgres, cfg.Storage.SQL.Dialect)
		require.Equal(t, "audit_events", cfg.Storage.SQL.TableName)
		require.Equal(t, 500, cfg.Storage.SQL.MaxBatchRows)
		require.Equal(t, time.Minute, cfg.Storage.SQL.PurgeInterval)
		require.Equal(t, 1000, cfg.Storage.SQL.PurgeBatchSize)
		require.NoError(t, cfg.Validate())
	})

	memDefaults := auditconfig.DefaultStorageMemory()
	require.Equal(t, auditconfig.DefaultMemoryMaxEvents, memDefaults.MaxEvents)
	require.NoError(t, memDefaults.Validate())
}
