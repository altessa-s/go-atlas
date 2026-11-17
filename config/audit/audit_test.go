// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auditconfig_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/types/redacted"

	auditconfig "github.com/altessa-s/go-atlas/config/audit"
	dispatchconfig "github.com/altessa-s/go-atlas/config/dispatch"
	pagingconfig "github.com/altessa-s/go-atlas/config/paging"
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
				TableName:           "audit_events",
				Engine:              "ReplacingMergeTree",
				MaxBatchSize:        10_000,
				DDLTimeout:          30 * time.Second,
				TimeRangeMode:       "warn",
				SchemaCheckMode:     "warn",
				SchemaMigrationMode: "off",
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
		{"empty storage type", func(c *auditconfig.Config) { c.Storage.Type = "" }, true},
		{"memory needs no block", func(c *auditconfig.Config) {
			c.Storage = auditconfig.Storage{Type: auditconfig.StorageTypeMemory}
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
		{"clickhouse unknown migration mode", func(c *auditconfig.Config) {
			c.Storage.ClickHouse.SchemaMigrationMode = "destructive"
		}, true},
		{"clickhouse empty modes use defaults", func(c *auditconfig.Config) {
			ch := c.Storage.ClickHouse
			ch.TimeRangeMode, ch.SchemaCheckMode, ch.SchemaMigrationMode = "", "", ""
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
