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
// — the timestamp at full precision. [New] leaves schema creation to the caller: call
// [Storage.EnsureSchema] once at startup, or apply the same DDL through a
// migration tool. IDs and filters compare byte-wise: COLLATE "C" on
// PostgreSQL, binary columns on MySQL/MariaDB. A queryable field longer than
// [MaxFilterBytes] bytes, or an ID longer than [MaxIDLength] characters, fails
// the batch with [ErrValueTooLong] before anything is written.
//
// # Writes
//
// StoreBatch rejects a nil event before writing. A storage-owned batch is atomic:
// one INSERT, or INSERTs of at most [WithMaxBatchRows] rows in one transaction.
// A caller owning the transaction must roll it back on error. Inserts are
// idempotent — an event whose ID is already stored is skipped (ON CONFLICT DO NOTHING on PostgreSQL, a no-op ON
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
// [WithTTL] enables retention; zero (the default) keeps events forever, as in
// mongo. SQL has no TTL index: from the first [Storage.Store] or
// [Storage.StoreBatch] until [Storage.Close], the storage runs background
// deletes every [DefaultPurgeInterval] (one minute), starting after the first
// interval; [New] starts no goroutine, and reads do not start the loop.
// [WithPurgeInterval] and [WithPurgeBatchSize] tune this loop; a zero interval
// disables background work while manual purging remains available. Each pass drains
// rows older than the application clock minus TTL through the (ts_ms, id)
// index, committing at most [DefaultPurgeBatchSize] rows per statement by
// default. Each statement is bounded by a 30-second timeout. PostgreSQL uses
// FOR UPDATE SKIP LOCKED in the ID subselect so concurrent purges skip busy
// rows. Background contexts carry no request values or transactions.
// [WithClock] injects the cutoff clock; interval timers use real time.
// Expiration is asynchronous: rows remain queryable until deleted. Failures
// and recovered panics are logged through [WithLogger] and retried next pass.
//
// [Storage.PurgeExpired] also supports explicit retention calls and returns
// completed deletions even when a later batch fails. [Storage.Close] stops
// the background loop for good, cancels its in-flight delete and waits within
// its context for the loop to exit, without closing the caller's database.
// Repeated Close calls wait again; an exited loop satisfies an ended context.
// Explicit retention
// calls use their own contexts and remain available after Close. Unlike a
// MongoDB TTL index, the SQL background loop cannot expire rows while closed.
//
// # Usage
//
//	db, _ := sql.Open("pgx", dsn)
//	storage, err := sqldb.New(db, sqldb.DialectPostgres)
//	if err != nil {
//		return err
//	}
//	defer storage.Close(ctx)
//	if err := storage.EnsureSchema(ctx); err != nil { // or apply the DDL via migrations
//		return err
//	}
//	eng, _ := dispatch.NewEngine[*audit.Event](audit.StorageSink{Storage: storage})
package sqldb
