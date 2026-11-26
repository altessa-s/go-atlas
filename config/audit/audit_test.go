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
		{"empty storage type with clickhouse block", func(c *auditconfig.Config) { c.Storage.Type = "" }, true},
		{"empty storage type with mongo block", func(c *auditconfig.Config) {
			c.Storage = auditconfig.Storage{Mongo: &auditconfig.StorageMongo{}}
		}, true},
		{"empty storage type with sql block", func(c *auditconfig.Config) {
			c.Storage = auditconfig.Storage{SQL: &auditconfig.StorageSQL{Dialect: auditconfig.SQLDialectPostgres, TableName: "audit_events", PurgeBatchSize: 1}}
		}, true},
		{"omitted storage uses default memory", func(c *auditconfig.Config) {
			c.Storage = auditconfig.Storage{}
		}, false},
		{"omitted storage type with memory block", func(c *auditconfig.Config) {
			c.Storage = auditconfig.Storage{Memory: &auditconfig.StorageMemory{MaxEvents: 500}}
		}, false},
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

	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("audit:\n  enabled: true\n"), 0o600))

	var root struct {
		Audit auditconfig.Config `yaml:"audit"`
	}
	_, err := loader.New(nil, loader.WithPath(path), loader.WithSkipEnv()).Load(&root)
	require.NoError(t, err)
	require.Empty(t, root.Audit.Storage.Type, "loader must leave omitted storage.type empty so factory/audit can warn on default memory fallback")
	require.NoError(t, root.Audit.Validate())

	memDefaults := auditconfig.DefaultStorageMemory()
	require.Equal(t, auditconfig.DefaultMemoryMaxEvents, memDefaults.MaxEvents)
	require.NoError(t, memDefaults.Validate())
}
