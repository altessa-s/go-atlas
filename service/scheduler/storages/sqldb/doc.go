// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package sqldb implements [scheduler.Storage] on a SQL database through the
// standard database/sql package. PostgreSQL 12+ ([DialectPostgres]) and MySQL
// 8.0+ / MariaDB 10.6+ ([DialectMySQL]) are supported. The caller owns the
// *sql.DB and chooses the driver — the toolkit itself depends on no SQL
// driver.
//
// # Schema
//
// Task states and execution history live in two tables, defaulting to
// [DefaultTasksTable] and [DefaultHistoryTable]. [New] performs no I/O: call
// [Storage.EnsureSchema] once at startup, or apply the same DDL through a
// migration tool; it also adds the run-lease and occurrence columns to a tasks
// table created by an earlier release, safely from several instances at once.
// Every string column compares exactly: IDs that differ only by case or
// trailing spaces are distinct rows, and run ownership is case-sensitive. On
// MySQL/MariaDB the string columns are binary types (VARBINARY, MEDIUMBLOB,
// LONGBLOB), so the schema never depends on the server version or the
// database's default character set; tables created by an earlier release with
// NO PAD binary collations are equally exact and are left unchanged.
//
// # Atomicity
//
// UpsertTask, CreateTask, ClaimRun, RenewRun, FinishRun and ReplaceTaskIf are
// each one statement, conditional where the run-ownership rules of
// [scheduler.Storage] fence it, that also sets the revision, so the database
// row lock serializes concurrent schedulers. DeleteTask removes a task and its
// history in one transaction.
//
// # Filters
//
// TasksPaginated and HistoryPaginated translate CEL filters with the
// data/filter PostgreSQL and MariaDB translators and evaluate them in the
// database. On MySQL/MariaDB the string fields are filtered through a
// case-sensitive utf8mb4_bin text view of the binary columns, so size(),
// endsWith() and matches() see characters; that collation is PAD SPACE, so
// comparisons and in (not the string functions) in a filter ignore trailing
// spaces there.
//
// # Usage
//
//	db, _ := sql.Open("pgx", dsn)
//	storage, err := sqldb.New(db, sqldb.DialectPostgres)
//	if err != nil {
//		return err
//	}
//	if err := storage.EnsureSchema(ctx); err != nil {
//		return err
//	}
//	s := scheduler.New(storage)
package sqldb
