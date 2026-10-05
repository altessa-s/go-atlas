// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package sqldb implements [scheduler.Storage] on a SQL database through the
// standard database/sql package. PostgreSQL 12+ ([DialectPostgres]) and MySQL
// 8.0.17+ / MariaDB 10.6+ ([DialectMySQL]) are supported. The caller owns the
// *sql.DB and chooses the driver — the toolkit itself depends on no SQL
// driver.
//
// # Schema
//
// Task states and execution history live in two tables, defaulting to
// [DefaultTasksTable] and [DefaultHistoryTable]. Call [Storage.EnsureSchema]
// once at startup, or apply the same DDL through a migration tool. Every string
// column compares exactly: IDs that differ only by case or trailing spaces are
// distinct rows, and run ownership is case-sensitive. On MySQL/MariaDB the
// schema declares utf8mb4 with a NO PAD binary collation per engine, so it never
// inherits database defaults.
//
// # Atomicity
//
// UpsertTask, ClaimRun, FinishRun and ReplaceTaskIf are each one conditional
// statement that also increments the revision, so the database row lock
// serializes concurrent schedulers. DeleteTask removes a task and its history in
// one transaction.
//
// # Filters
//
// TasksPaginated and HistoryPaginated translate CEL filters with the
// data/filter PostgreSQL and MariaDB translators and evaluate them in the
// database.
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
