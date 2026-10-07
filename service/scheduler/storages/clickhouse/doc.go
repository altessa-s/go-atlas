// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package clickhouse keeps the scheduler's execution history in a ClickHouse
// table. It implements [scheduler.HistoryStorage] — not [scheduler.Storage]:
// task state needs atomic compare-and-swap writes that ClickHouse does not
// offer, so it stays in another backend, and [scheduler.WithHistoryStorage]
// moves only the history here. History suits ClickHouse well: it is only
// appended, listed per task and expired.
//
// # Schema
//
// [SchemaDDL] renders the table: partitioned by the month an entry ended,
// sorted by (task_id, started_at, id), with a TTL of [WithTTL] after the end of
// a run. [Storage.EnsureSchema] creates it unless it exists; [New] performs no
// I/O, so a migration-managed deployment applies the DDL itself. Table, cluster
// and engine names are validated, failing with [ErrInvalidIdentifier] or
// [ErrInvalidEngine].
//
// # Retention
//
// The table TTL is the retention: [Storage.CleanupHistory] is a no-op, and the
// scheduler's WithHistoryRetention does not apply. ClickHouse deletes expired
// rows as it merges parts, so an expired entry stays visible until then. The
// TTL is fixed when the table is created; change it with ALTER TABLE … MODIFY
// TTL. A row already past its TTL when written is dropped on insert.
//
// # Writes and deletes
//
// The scheduler records one entry per run, so [Storage.AddHistory] uses an
// asynchronous insert, which the server buffers into shared blocks instead of
// creating a part per row; the call waits for the flush.
// [Storage.DeleteHistory], called by Scheduler.Unregister, is a lightweight
// DELETE that waits for every replica (lightweight_deletes_sync = 2, ClickHouse
// 24.x) whatever the session default.
//
// # Filters
//
// [Storage.HistoryPaginated] translates filters with
// data/filter/translators/clickhouse over [scheduler.HistoryFilterFields]. Its
// size() is length(), which measures a string in bytes rather than code points.
//
// # Basic Usage
//
//	conn, _ := chfactory.New(cfg).Build(ctx) // infrastructure/clickhouse/factory
//	history, err := clickhouse.New(conn, clickhouse.WithTTL(30*24*time.Hour))
//	if err != nil {
//	    log.Fatal(err)
//	}
//	if err := history.EnsureSchema(ctx); err != nil {
//	    log.Fatal(err)
//	}
//	s := scheduler.New(tasks, scheduler.WithHistoryStorage(history))
package clickhouse
