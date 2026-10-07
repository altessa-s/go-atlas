// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package clickhouse provides a ClickHouse-backed [audit.Storage].
//
// Audit traffic is append-only, high-volume, and read back analytically —
// the workload ClickHouse is built for. Events land in a single wide table,
// partitioned by month and sorted so that the common query shape (a time
// range, then an actor) reads as few granules as possible.
//
// The connection is injected and stays owned by the caller:
//
//	conn, err := clickhouse.Open(&clickhouse.Options{Addr: []string{"ch:9000"}})
//	if err != nil {
//		return err
//	}
//	defer conn.Close()
//
//	storage, err := auditclickhouse.New(conn,
//		auditclickhouse.WithTableName("audit_events"),
//		auditclickhouse.WithTimeRangeMode(auditclickhouse.TimeRangeModeEnforce),
//	)
//	if err != nil {
//		return err
//	}
//
//	if err := storage.StoreBatch(ctx, events); err != nil {
//		return err
//	}
//
// # Schema
//
// Fields that queries filter on become typed columns; free-form maps
// (metadata, attributes, resource changes) are carried as JSON strings so
// the table stays fixed-width without a schema per tenant. [SchemaDDL]
// renders the CREATE TABLE statement so it can be applied by a migration
// rather than by the process writing events; [WithAutoCreateTable] runs it
// from [New] instead, which suits tests and single-tenant deployments. The
// table, cluster and engine names reach SQL unescaped, so both validate
// them: a plain identifier, a cluster name, and a MergeTree-family engine
// whose parameters are literals.
//
// # Writing
//
// [Storage.StoreBatch] is the hot path: it splits its input into batches of
// [DefaultMaxBatchSize] rows and sends each as one INSERT. [Storage.Store]
// exists for the occasional lone event and routes it through an
// asynchronous insert, since a single-row INSERT would otherwise create a
// part of its own.
//
// # Deduplication
//
// The dispatcher delivers at-least-once, so a crash-and-replay sequence can
// present the same event twice. The default ReplacingMergeTree engine
// collapses those duplicates by the sorting key, but only once the parts
// merge. [audit.FetchPage] returns each (timestamp, id) once regardless;
// raw [Storage.Query] and [Storage.Count] need [WithFinal] to stop seeing a
// transient duplicate.
//
// # Retention
//
// Retention is declarative — a positive TTL on the table drops expired rows
// during merges, and monthly partitions make bulk deletion cheap. Removing
// an individual subject's events is an ALTER TABLE ... DELETE mutation,
// which ClickHouse applies asynchronously by rewriting parts.
package clickhouse
