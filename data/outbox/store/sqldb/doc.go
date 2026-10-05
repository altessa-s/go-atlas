// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package sqldb implements [outbox.Store] on a SQL database through the
// standard database/sql package: PostgreSQL 12+ ([DialectPostgres]) and MySQL
// 8.0+ / MariaDB 10.6+ ([DialectMySQL]). The caller owns the *sql.DB and
// chooses the driver — the toolkit itself depends on no SQL driver.
//
// # Schema
//
// [New] performs no I/O. Call [Store.EnsureSchema] once at startup to create
// the events table and its indexes if they do not exist — it is idempotent —
// or apply the same DDL through a migration tool. On MySQL/MariaDB the string
// columns are binary types, so storage and comparison never depend on the
// database's default character set.
//
// # Transactions
//
// The outbox is only worth having if an event commits together with the
// business data it describes. Pass the business transaction to Save through
// [WithTx]:
//
//	tx, err := db.BeginTx(ctx, nil)
//	// ... business writes on tx ...
//	if err := ob.Save(sqldb.WithTx(ctx, tx), outbox.Event{Key: "orders.created", Payload: data}); err != nil {
//		_ = tx.Rollback()
//		return err
//	}
//	return tx.Commit()
//
// # Server clock
//
// Every time-based predicate — retry backoff, lock expiry, retention,
// expiration — is evaluated against the database clock (now() on PostgreSQL,
// UTC_TIMESTAMP(6) on MySQL), and the timestamps those predicates compare
// against are stamped by it too. On MySQL instants cross the wire as UTC
// strings rather than time.Time, so the driver's loc and parseTime settings
// cannot shift them.
//
// # Locking
//
// FetchUnprocessedEvents selects a batch FOR UPDATE SKIP LOCKED and marks it
// in-progress with a fresh lock token in one transaction, so concurrent
// dispatchers take disjoint batches. UpdateEvents writes only while the stored
// token still matches, fencing out a dispatcher whose lease was reclaimed.
//
// # Watch
//
// database/sql has no notification API, so the store does not implement
// [outbox.Watcher]; dispatch runs on the poll schedule.
//
// # Usage
//
//	db, _ := sql.Open("pgx", dsn)
//	store, err := sqldb.New(db, sqldb.DialectPostgres)
//	if err != nil {
//		return err
//	}
//	if err := store.EnsureSchema(ctx); err != nil { // or apply the DDL via migrations
//		return err
//	}
//	ob := outbox.New(store, handler, outbox.WithScheduler(sched))
package sqldb
