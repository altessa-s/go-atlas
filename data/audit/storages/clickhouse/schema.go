// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clickhouse

import (
	"fmt"
	"strings"
	"time"
)

// Column names of the audit events table. They are the contract shared by
// [SchemaDDL], the INSERT statement, and the query builder — renaming one
// here is a schema migration, not a refactor.
const (
	colTimestamp = "timestamp"
	colID        = "id"
	colType      = "type"
	colAction    = "action"

	colActorType      = "actor_type"
	colActorID        = "actor_id"
	colActorName      = "actor_name"
	colActorEmail     = "actor_email"
	colActorIP        = "actor_ip"
	colActorUserAgent = "actor_user_agent"
	colActorRoles     = "actor_roles"
	colActorMetadata  = "actor_metadata"

	colResourceType          = "resource_type"
	colResourceID            = "resource_id"
	colResourceName          = "resource_name"
	colResourcePath          = "resource_path"
	colResourceChangesBefore = "resource_changes_before"
	colResourceChangesAfter  = "resource_changes_after"
	colResourceChangesFields = "resource_changes_fields"
	colResourceAttributes    = "resource_attributes"

	colResultStatus       = "result_status"
	colResultCode         = "result_code"
	colResultMessage      = "result_message"
	colResultErrorCode    = "result_error_code"
	colResultErrorMessage = "result_error_message"

	colRequestID     = "request_id"
	colTraceID       = "trace_id"
	colSpanID        = "span_id"
	colCorrelationID = "correlation_id"

	colServiceName     = "service_name"
	colServiceVersion  = "service_version"
	colServiceInstance = "service_instance"

	colDurationNS = "duration_ns"
	colMetadata   = "metadata"
)

// Table-level clauses. The ORDER BY prefix mirrors the expected query shape:
// audit reads are almost always bounded by a time range and then narrowed by
// actor. Changing it invalidates existing parts and requires a rebuild.
const (
	partitionByExpr = "toYYYYMM(" + colTimestamp + ")"
	orderByExpr     = "(toDate(" + colTimestamp + "), " + colActorID + ", " + colTimestamp + ", " + colID + ")"
)

// columnDef is one column of the audit events table.
type columnDef struct {
	name   string
	chType string
}

// skipIndexDef is a data-skipping index on a column outside the sorting key.
type skipIndexDef struct {
	name        string
	column      string
	indexType   string
	granularity int
}

// Skip-index tuning. A 1% false-positive rate keeps the filter small while
// still discarding the overwhelming majority of granules, and a granularity
// of 4 means one filter per 4 index granules — coarse enough to stay cheap
// on write, fine enough to skip most of a partition on a point lookup.
const (
	skipIndexFalsePositiveRate = "bloom_filter(0.01)"
	skipIndexGranularity       = 4
)

// skipIndexes cover the lookups that the sorting key cannot serve: the two
// correlation identifiers, and the resource an event acted on. Each sits
// outside the sorting key, so tracing a request or listing one object's
// history would otherwise read every granule of every partition in range. A
// bloom filter turns that into a cheap membership test per granule; the
// write cost is one small structure per index granule.
var skipIndexes = []skipIndexDef{
	{name: "idx_trace_id", column: colTraceID, indexType: skipIndexFalsePositiveRate, granularity: skipIndexGranularity},
	{name: "idx_request_id", column: colRequestID, indexType: skipIndexFalsePositiveRate, granularity: skipIndexGranularity},
	{name: "idx_resource_id", column: colResourceID, indexType: skipIndexFalsePositiveRate, granularity: skipIndexGranularity},
}

// schemaColumns lists every column in INSERT order. [eventRow.args] and
// [eventRow.scanDest] must yield values in exactly this order; the invariant
// is pinned by the tests.
var schemaColumns = []columnDef{
	{colTimestamp, "DateTime64(3, 'UTC')"},
	{colID, "String"},
	{colType, "LowCardinality(String)"},
	{colAction, "LowCardinality(String)"},

	{colActorType, "LowCardinality(String)"},
	{colActorID, "String"},
	{colActorName, "String"},
	{colActorEmail, "String"},
	{colActorIP, "String"},
	{colActorUserAgent, "String"},
	{colActorRoles, "Array(LowCardinality(String))"},
	{colActorMetadata, "String"},

	{colResourceType, "LowCardinality(String)"},
	{colResourceID, "String"},
	{colResourceName, "String"},
	{colResourcePath, "String"},
	{colResourceChangesBefore, "String"},
	{colResourceChangesAfter, "String"},
	{colResourceChangesFields, "Array(String)"},
	{colResourceAttributes, "String"},

	{colResultStatus, "LowCardinality(String)"},
	{colResultCode, "Int64"},
	{colResultMessage, "String"},
	{colResultErrorCode, "LowCardinality(String)"},
	{colResultErrorMessage, "String"},

	{colRequestID, "String"},
	{colTraceID, "String"},
	{colSpanID, "String"},
	{colCorrelationID, "String"},

	{colServiceName, "LowCardinality(String)"},
	{colServiceVersion, "LowCardinality(String)"},
	{colServiceInstance, "LowCardinality(String)"},

	{colDurationNS, "Int64"},
	{colMetadata, "String"},
}

// SchemaDDL returns the CREATE TABLE statement backing the audit storage.
//
// The table is partitioned by month and sorted by (date, actor, timestamp,
// id), and carries bloom-filter skip indexes on the correlation ids. A
// positive ttl adds a row-level TTL clause; a non-positive ttl omits it, and
// events are then retained indefinitely. TTL has second granularity: a
// sub-second ttl is raised to one second rather than truncated to zero,
// which ClickHouse would read as "expire on insert".
//
// A non-empty cluster adds an ON CLUSTER clause, which is what a replicated
// deployment needs — pair it with a Replicated* engine, since ON CLUSTER
// alone only means "run this DDL on every node", not "replicate the data".
//
// engine is emitted as the ENGINE clause, so it may carry parameters (for
// example "ReplicatedReplacingMergeTree('/clickhouse/tables/{shard}/audit', '{replica}')").
// The names are validated rather than escaped: the table must be a plain
// identifier, the cluster a cluster name, and the engine a MergeTree-family
// engine whose parameters are string literals, identifiers or numbers.
// Anything else fails with [ErrInvalidIdentifier] or [ErrInvalidEngine].
func SchemaDDL(table, engine, cluster string, ttl time.Duration) (string, error) {
	if err := validateSchemaNames(table, engine, cluster); err != nil {
		return "", err
	}

	var sb strings.Builder

	fmt.Fprintf(&sb, "CREATE TABLE IF NOT EXISTS `%s`", table)
	if cluster != "" {
		fmt.Fprintf(&sb, " ON CLUSTER `%s`", cluster)
	}
	sb.WriteString(" (\n")

	for _, c := range schemaColumns {
		fmt.Fprintf(&sb, "    `%s` %s,\n", c.name, c.chType)
	}

	for i, idx := range skipIndexes {
		sep := ","
		if i == len(skipIndexes)-1 {
			sep = ""
		}
		fmt.Fprintf(&sb, "    INDEX %s `%s` TYPE %s GRANULARITY %d%s\n",
			idx.name, idx.column, idx.indexType, idx.granularity, sep)
	}

	fmt.Fprintf(&sb, ")\nENGINE = %s", engine)
	fmt.Fprintf(&sb, "\nPARTITION BY %s", partitionByExpr)
	fmt.Fprintf(&sb, "\nORDER BY %s", orderByExpr)

	if ttl > 0 {
		fmt.Fprintf(&sb, "\nTTL toDateTime(%s) + INTERVAL %d SECOND", colTimestamp, max(int64(ttl/time.Second), 1))
	}

	return sb.String(), nil
}
