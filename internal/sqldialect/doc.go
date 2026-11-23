// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package sqldialect holds SQL execution and text helpers shared by the database/sql
// storages (reliability/outbox/storages/sqldb, reliability/saga/storages/sqldb, ...):
// table-name validation and quoting, index naming, placeholder binding, and
// serialized PostgreSQL schema creation. [Pool] executes standalone PostgreSQL
// statements with one retry for a driver-confirmed pre-send connection failure.
//
// # Identifiers
//
// Table names are the only SQL fragments a storage cannot bind as parameters,
// so [Style.Table] accepts only a plain identifier, optionally qualified by one
// schema name, with every part at most [MaxIdentLen] characters, and quotes
// each part. [Style.IndexName] derives index names from the unqualified table
// name and hashes the tail of a name that would exceed the limit.
//
// # Placeholders
//
// Statements are written with ? placeholders; [Style.Bind] renumbers them as
// $1, $2 … for PostgreSQL, starting from any index so a statement can follow a
// translated filter that owns the first placeholders.
//
// # Schema creation
//
// [ExecPostgresDDL] runs a storage's DDL in one PostgreSQL transaction under
// advisory locks on its table names, so instances creating the same absent
// tables at once take turns instead of colliding in the catalog.
// [MySQLErrorNumber] reads server error numbers without a SQL
// driver dependency, including joined error trees. [IsDuplicateKey] recognizes
// PostgreSQL SQLSTATE 23505 and MySQL error number 1062 by driver error shape.
// [QueryKeys] and [InList] serve the MySQL sweeps that select a batch of
// keys and then delete them by key.
//
// # Errors
//
// [ErrUnsupportedDialect], [ErrInvalidTableName] and [ErrValueTooLong] are
// re-exported by the applicable storage packages, so errors.Is matches across them.
//
// # Usage
//
//	table, err := sqldialect.Postgres.Table("app.events")
//	if err != nil {
//		return err
//	}
//	query := sqldialect.Postgres.Bind("SELECT id FROM "+table+" WHERE id = ?", 1)
package sqldialect
