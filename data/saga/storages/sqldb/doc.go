// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package sqldb implements [saga.Storage] on a SQL database through the
// standard database/sql package: PostgreSQL 12+ ([DialectPostgres]) and MySQL
// 8.0+ / MariaDB 10.6+ ([DialectMySQL]). The caller owns the *sql.DB and
// chooses the driver — the toolkit itself depends on no SQL driver.
//
// # Schema
//
// Each saga instance is one row of a table defaulting to [DefaultTableName].
// [New] performs no I/O: call [Store.EnsureSchema] once at startup — it is
// idempotent and safe to run from several instances at once — or apply the
// same DDL through a migration tool. IDs compare exactly: IDs that differ only
// by case or a trailing space are distinct rows. On MySQL/MariaDB the string
// columns are binary types, so the schema never depends on the database's
// default character set. Pending steps and step records are stored as JSON,
// Data as opaque bytes.
//
// # Concurrency
//
// Create is insert-if-absent: ON CONFLICT DO NOTHING on PostgreSQL, a plain
// INSERT on MySQL whose duplicate-key error (1062, read from the message as
// go-sql-driver/mysql renders it) is reported as [sagaerrs.ErrInstanceExists];
// any other error is returned as is. Update is one UPDATE conditional on the
// stored version,
// so the database row lock serializes concurrent coordinators and exactly one
// of them advances an instance.
//
// # Time
//
// Timestamps are Unix nanoseconds, 0 for the zero time, so LeaseUntil and
// Deadline are compared at full precision against the now passed to
// FetchRecoverable — the caller's clock, as with every other backend. Times
// outside the years 1678–2262 do not fit.
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
//	orch := saga.New(store, def)
package sqldb
