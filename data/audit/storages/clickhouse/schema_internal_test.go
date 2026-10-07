// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clickhouse

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func mustDDL(t testing.TB, table, engine, cluster string, ttl time.Duration) string {
	t.Helper()
	ddl, err := SchemaDDL(table, engine, cluster, ttl)
	require.NoError(t, err)
	return ddl
}

func TestSchemaDDLRejectsUnsafeNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		table, engine, cluster string
		want                   error
	}{
		{"table backtick", "audit`; DROP TABLE x; --", "MergeTree", "", ErrInvalidIdentifier},
		{"table qualified", "db.audit", "MergeTree", "", ErrInvalidIdentifier},
		{"table empty", "", "MergeTree", "", ErrInvalidIdentifier},
		{"table comment", "audit--x", "MergeTree", "", ErrInvalidIdentifier},
		{"cluster backtick", "audit", "MergeTree", "prod` SETTINGS x=1 --", ErrInvalidIdentifier},
		{"cluster space", "audit", "MergeTree", "prod main", ErrInvalidIdentifier},
		{"engine trailing clause", "audit", "MergeTree SETTINGS index_granularity=1", "", ErrInvalidEngine},
		{"engine statement", "audit", "MergeTree; DROP TABLE audit", "", ErrInvalidEngine},
		{"engine comment", "audit", "MergeTree -- x", "", ErrInvalidEngine},
		{"engine quote escape", "audit", `ReplicatedMergeTree('a\', 'b')`, "", ErrInvalidEngine},
		{"engine expression arg", "audit", "ReplacingMergeTree(now())", "", ErrInvalidEngine},
		{"engine other family", "audit", "Log", "", ErrInvalidEngine},
		{"engine empty", "audit", "", "", ErrInvalidEngine},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := SchemaDDL(tt.table, tt.engine, tt.cluster, 0)
			require.ErrorIs(t, err, tt.want)

			if tt.table == "" || tt.engine == "" {
				return // the options ignore empty values and keep the defaults
			}
			_, err = New(&mockConn{}, WithTableName(tt.table), WithEngine(tt.engine), WithCluster(tt.cluster))
			require.ErrorIs(t, err, tt.want)
		})
	}
}

func TestSchemaDDLAcceptsEngines(t *testing.T) {
	t.Parallel()

	for _, engine := range []string{
		"MergeTree",
		"ReplacingMergeTree",
		"ReplacingMergeTree(inserted_at)",
		"ReplicatedReplacingMergeTree('/clickhouse/tables/{shard}/audit', '{replica}')",
		"ReplicatedReplacingMergeTree( '/t/{shard}' , '{replica}', ver )",
		"SharedMergeTree()",
	} {
		_, err := SchemaDDL("audit_events", engine, "prod-eu.1", 0)
		require.NoError(t, err, engine)
	}
}

func TestSchemaDDLClauses(t *testing.T) {
	t.Parallel()

	ddl := mustDDL(t, "audit_events", "ReplacingMergeTree", "", 0)

	require.True(t, strings.HasPrefix(ddl, "CREATE TABLE IF NOT EXISTS `audit_events` (\n"), ddl)
	require.Contains(t, ddl, "\n)\nENGINE = ReplacingMergeTree\n")
	require.Contains(t, ddl, "\nPARTITION BY toYYYYMM(timestamp)")
	require.Contains(t, ddl, "\nORDER BY (toDate(timestamp), actor_id, timestamp, id)")
	require.NotContains(t, ddl, "TTL")
	require.NotContains(t, ddl, "ON CLUSTER")
}

func TestSchemaDDLEngineIsVerbatim(t *testing.T) {
	t.Parallel()

	ddl := mustDDL(t, "events", "ReplacingMergeTree(inserted_at)", "", 0)

	require.Contains(t, ddl, "ENGINE = ReplacingMergeTree(inserted_at)")
}

func TestSchemaDDLOnCluster(t *testing.T) {
	t.Parallel()

	ddl := mustDDL(t, "audit_events", "ReplicatedReplacingMergeTree('/tables/audit', '{replica}')", "prod", 0)

	require.True(t, strings.HasPrefix(ddl, "CREATE TABLE IF NOT EXISTS `audit_events` ON CLUSTER `prod` (\n"), ddl)
	require.Contains(t, ddl, "ENGINE = ReplicatedReplacingMergeTree('/tables/audit', '{replica}')")
}

func TestSchemaDDLSkipIndexes(t *testing.T) {
	t.Parallel()

	ddl := mustDDL(t, "audit_events", "MergeTree", "", 0)

	require.Contains(t, ddl, "    INDEX idx_trace_id `trace_id` TYPE bloom_filter(0.01) GRANULARITY 4,\n")
	require.Contains(t, ddl, "    INDEX idx_request_id `request_id` TYPE bloom_filter(0.01) GRANULARITY 4,\n")
	require.Contains(t, ddl, "    INDEX idx_resource_id `resource_id` TYPE bloom_filter(0.01) GRANULARITY 4\n")

	// The last element of the block carries no separator, or ClickHouse
	// rejects the statement.
	require.NotContains(t, ddl, ",\n)")
}

func TestSchemaDDLTTL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ttl  time.Duration
		want string // empty means no TTL clause at all
	}{
		{name: "disabled", ttl: 0},
		{name: "negative", ttl: -time.Hour},
		{name: "days", ttl: 30 * 24 * time.Hour, want: "TTL toDateTime(timestamp) + INTERVAL 2592000 SECOND"},
		{name: "truncated to whole seconds", ttl: 90*time.Second + 500*time.Millisecond, want: "TTL toDateTime(timestamp) + INTERVAL 90 SECOND"},
		{name: "sub-second raised to one", ttl: 500 * time.Millisecond, want: "TTL toDateTime(timestamp) + INTERVAL 1 SECOND"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ddl := mustDDL(t, "audit_events", "MergeTree", "", tt.ttl)

			if tt.want == "" {
				require.NotContains(t, ddl, "TTL")
				return
			}
			require.Contains(t, ddl, tt.want)
		})
	}
}

func TestSchemaDDLColumns(t *testing.T) {
	t.Parallel()

	ddl := mustDDL(t, "audit_events", "MergeTree", "", 0)

	// Every column carries a separator now: the index block follows them.
	for _, c := range schemaColumns {
		require.Contains(t, ddl, fmt.Sprintf("    `%s` %s,\n", c.name, c.chType), "column %s", c.name)
	}
}

func TestSchemaColumnNamesAreUnique(t *testing.T) {
	t.Parallel()

	seen := make(map[string]struct{}, len(schemaColumns))
	for _, c := range schemaColumns {
		require.NotEmpty(t, c.chType, "column %s has no type", c.name)
		_, dup := seen[c.name]
		require.False(t, dup, "duplicate column %s", c.name)
		seen[c.name] = struct{}{}
	}
}
