// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package sqldb implements [retryqueue.Storage] and [retryqueue.Mover] on a
// SQL database through the standard database/sql package: PostgreSQL 12+
// ([DialectPostgres]) and MySQL 8.0+ / MariaDB 10.6+ ([DialectMySQL]). The
// caller owns the *sql.DB and chooses the driver — the toolkit itself depends
// on no SQL driver.
//
// # Schema
//
// Each item is one row of a table defaulting to [DefaultTableName], keyed by
// the item key. [New] performs no I/O: call [Store.EnsureSchema] once at
// startup — it is idempotent and safe to run from several instances at once —
// or apply the same DDL through a migration tool. Keys compare exactly and
// sort byte-wise; on MySQL/MariaDB the string columns are binary types, so the
// schema never depends on the database's default character set. The payload
// is encoded by the configured codec ([WithCodec], JSON by default).
// EnsureSchema creates the secondary indexes with the table.
//
// # Concurrency
//
// Every transition is one conditional statement fenced by generation (and,
// for Expire, by age), so the database row lock serializes racing callers and
// exactly one of them makes a transition. Mark is insert-if-absent: ON
// CONFLICT DO NOTHING on PostgreSQL, a plain INSERT whose duplicate-key error
// is absorbed on MySQL. Move renames the primary key in one UPDATE.
//
// # Transactions
//
// [WithTx] puts a caller transaction on the context; every method then runs
// on it and commits or rolls back with the caller's other writes.
// Requeue splits key lists into chunks of at most 1000 keys and runs all
// chunks in one transaction, starting one when no caller transaction exists.
//
// # Time
//
// Instants are the caller's, stored as Unix nanoseconds at full precision, 0
// for the zero time; times outside the years 1678–2262 do not fit. Resolved
// items are kept until [Store.PurgeExpired], which compares against the
// database clock at whole seconds and deletes those past the resolved
// retention ([WithResolvedRetention]). ResolvedAt is the instant the caller
// passed to Resolve (its clock), so application-to-database clock skew shifts
// the effective retention by that skew. Call PurgeExpired periodically, or
// let the store run it in the background with [WithPurgeInterval] from its
// first write call until [Store.Close].
//
// # Usage
//
//	db, _ := sql.Open("pgx", dsn)
//	queue, err := sqldb.New[Valuation](db, sqldb.DialectPostgres, sqldb.WithResolvedRetention(7*24*time.Hour))
//	if err != nil {
//		return err
//	}
//	if err := queue.EnsureSchema(ctx); err != nil { // or apply the DDL via migrations
//		return err
//	}
//
// # Purging
//
// PurgeExpired(ctx) (int64, error) drains currently purgeable rows in batches and returns completed
// deletions even if a later batch fails. WithPurgeInterval defaults to one minute; zero disables the
// background loop while manual purging stays available. WithPurgeBatchSize defaults to 1000. New
// starts no goroutine; the first write starts the loop when retention is enabled. Close(ctx)
// permanently stops it and waits within the context, including on repeated calls. A loop that has
// already exited satisfies an ended context. Background failures and recovered panics go to WithLogger
// (discard by default). Each statement is bounded by a 30-second timeout.
// PostgreSQL deletes batches using FOR UPDATE SKIP LOCKED; MySQL orders them
// by resolved_at. Background contexts carry no request values or transactions;
// explicit purges still join WithTx.
package sqldb
