// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clickhouse

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ErrInvalidIdentifier is returned when the table or cluster name is not a
// plain identifier.
var ErrInvalidIdentifier = errors.New("scheduler/clickhouse: invalid identifier")

// ErrInvalidEngine is returned when the engine is not a MergeTree-family
// engine with literal parameters.
var ErrInvalidEngine = errors.New("scheduler/clickhouse: invalid engine")

var (
	// tableNamePattern admits an unqualified identifier; the name is quoted
	// with backticks wherever it reaches SQL.
	tableNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

	// clusterNamePattern also admits the dots and hyphens cluster names in
	// remote_servers commonly carry.
	clusterNamePattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)

	// enginePattern admits a MergeTree-family engine name, optionally with
	// parameters that are each a single-quoted string without quotes or
	// backslashes (ZooKeeper paths, {shard}/{replica} macros) or a bare
	// identifier or number.
	enginePattern = regexp.MustCompile(`^[A-Za-z]*MergeTree` +
		`(\(\s*(` + engineArg + `(\s*,\s*` + engineArg + `)*)?\s*\))?$`)
)

const engineArg = `('[^'\\]*'|[A-Za-z0-9_]+)`

// historyColumns lists the history table columns in scan order, the order
// [scanHistory] reads them in.
const historyColumns = "`id`, `task_id`, `run_id`, `error`, `started_at`, `ended_at`, `duration_ms`, `success`"

// SchemaDDL returns the CREATE TABLE statement backing the history storage.
//
// The table is partitioned by the month an entry ended and sorted by (task_id,
// started_at, id), which serves every read: each one is bounded to a single
// task and walks it by start time. Timestamps are Unix seconds, as in
// [scheduler.TaskHistory]. The TTL deletes an entry ttl after it ended — the
// storage's retention, in place of CleanupHistory. TTL has second granularity:
// a sub-second ttl is raised to one second. A non-positive ttl omits the
// clause, and history is then kept indefinitely.
//
// A non-empty cluster adds an ON CLUSTER clause; pair it with a Replicated*
// engine, since ON CLUSTER alone only distributes the DDL. engine is emitted as
// the ENGINE clause and may carry literal parameters. The names are validated
// rather than escaped: anything but a plain table identifier, a cluster name or
// a MergeTree-family engine fails with [ErrInvalidIdentifier] or
// [ErrInvalidEngine].
func SchemaDDL(table, engine, cluster string, ttl time.Duration) (string, error) {
	if err := validateSchemaNames(table, engine, cluster); err != nil {
		return "", err
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "CREATE TABLE IF NOT EXISTS `%s`%s (\n", table, onCluster(cluster))
	sb.WriteString("    `id` String,\n")
	sb.WriteString("    `task_id` String,\n")
	sb.WriteString("    `run_id` String,\n")
	sb.WriteString("    `error` String,\n")
	sb.WriteString("    `started_at` Int64,\n")
	sb.WriteString("    `ended_at` Int64,\n")
	sb.WriteString("    `duration_ms` Int64,\n")
	sb.WriteString("    `success` Bool\n")
	fmt.Fprintf(&sb, ")\nENGINE = %s", engine)
	sb.WriteString("\nPARTITION BY toYYYYMM(toDateTime(`ended_at`))")
	sb.WriteString("\nORDER BY (`task_id`, `started_at`, `id`)")
	if ttl > 0 {
		fmt.Fprintf(&sb, "\nTTL toDateTime(`ended_at`) + INTERVAL %d SECOND", max(int64(ttl/time.Second), 1))
	}
	return sb.String(), nil
}

// onCluster renders the ON CLUSTER clause of a statement, empty without a
// cluster.
func onCluster(cluster string) string {
	if cluster == "" {
		return ""
	}
	return " ON CLUSTER `" + cluster + "`"
}

// validateSchemaNames checks the names the statements of the storage
// interpolate into SQL. They come from configuration, so they are held to a
// grammar that leaves no room for anything but a name.
func validateSchemaNames(table, engine, cluster string) error {
	if !tableNamePattern.MatchString(table) {
		return fmt.Errorf("%w: table %q", ErrInvalidIdentifier, table)
	}
	if cluster != "" && !clusterNamePattern.MatchString(cluster) {
		return fmt.Errorf("%w: cluster %q", ErrInvalidIdentifier, cluster)
	}
	if !enginePattern.MatchString(engine) {
		return fmt.Errorf("%w: %q", ErrInvalidEngine, engine)
	}
	return nil
}
