// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package sqldb implements [audit.Storage] on a SQL database through the
// standard database/sql package: PostgreSQL 12+ ([DialectPostgres]) and MySQL
// 8.0+ / MariaDB 10.6+ ([DialectMySQL]), for deployments that keep audit
// events in the relational database they already run. ClickHouse remains the
// choice for analytical volumes. The caller owns the *sql.DB and chooses the
// driver — the toolkit itself depends on no SQL driver.
//
// # Schema
//
// Each event is one row: the ID, the timestamp in Unix milliseconds, the
// queryable fields (type, action, actor, resource, status, request and trace
// IDs) as columns, and the whole event as a JSON payload, which Query decodes
// — the timestamp at full precision. [New] performs no I/O: call
// [Storage.EnsureSchema] once at startup, or apply the same DDL through a
// migration tool. IDs and filters compare byte-wise: COLLATE "C" on
// PostgreSQL, binary columns on MySQL/MariaDB.
//
// # Writes
//
// StoreBatch is atomic: one INSERT, or INSERTs of at most [WithMaxBatchRows]
// rows in one transaction. Inserts are idempotent — an event whose ID is
// already stored is skipped (ON CONFLICT DO NOTHING on PostgreSQL, a no-op ON
// DUPLICATE KEY UPDATE on MySQL) — so the dispatcher's at-least-once retry of
// a batch whose commit acknowledgment was lost succeeds instead of failing on
// its own earlier write.
//
// # Reads
//
// Query orders by (timestamp millisecond, ID) and continues strictly after
// [audit.Query.Cursor], the order the page tokens of [audit.FetchPage] rely on;
// time bounds compare whole milliseconds. Metadata maps round-trip through
// JSON, so their numbers come back as float64.
//
// # Retention
//
// Rows are never expired by the storage; delete old events, or partition the
// table by time, through the database.
//
// # Usage
//
//	db, _ := sql.Open("pgx", dsn)
//	storage, err := sqldb.New(db, sqldb.DialectPostgres)
//	if err != nil {
//		return err
//	}
//	if err := storage.EnsureSchema(ctx); err != nil { // or apply the DDL via migrations
//		return err
//	}
//	eng, _ := dispatch.NewEngine[*audit.Event](audit.StorageSink{Storage: storage})
package sqldb
