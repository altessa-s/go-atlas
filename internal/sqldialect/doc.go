// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package sqldialect holds the SQL text helpers shared by the database/sql
// storages (service/scheduler/storages/sqldb and data/outbox/storages/sqldb):
// table-name validation and quoting, index naming, placeholder binding, and
// serialized PostgreSQL schema creation.
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
//
// # Errors
//
// [ErrUnsupportedDialect] and [ErrInvalidTableName] are re-exported by each
// storage package, so errors.Is matches across them.
//
// # Usage
//
//	table, err := sqldialect.Postgres.Table("app.events")
//	if err != nil {
//		return err
//	}
//	query := sqldialect.Postgres.Bind("SELECT id FROM "+table+" WHERE id = ?", 1)
package sqldialect
