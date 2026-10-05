// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqldialect

import (
	"context"
	"database/sql"
	"hash/fnv"
	"slices"
)

// lockNamespace prefixes advisory-lock names so the keys stay apart from the
// application's own advisory locks.
const lockNamespace = "go-atlas/sqldb:"

// ExecPostgresDDL runs stmts in one PostgreSQL transaction that first takes a
// transaction-scoped advisory lock per table name (in sorted order, so
// concurrent callers cannot deadlock). Pass unqualified names (see
// [Unqualified]): "events" and "public.events" may be the same table, so the
// lock ignores the schema — same-named tables in different schemas merely take
// turns. PostgreSQL's CREATE … IF NOT EXISTS checks the
// catalog without a lock, so two sessions creating the same absent table can
// both pass the check and one fails on a catalog unique index; holding the
// lock serializes them, and the later one finds the objects in place. DDL is
// transactional on PostgreSQL, so a failure leaves nothing half-created.
func ExecPostgresDDL(ctx context.Context, db *sql.DB, names []string, stmts []string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	keys := make([]int64, len(names))
	for i, name := range names {
		keys[i] = advisoryKey(name)
	}
	slices.Sort(keys)
	for _, key := range slices.Compact(keys) {
		if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", key); err != nil {
			return err
		}
	}
	for _, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// advisoryKey maps a name to a pg_advisory_xact_lock key.
func advisoryKey(name string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(lockNamespace + name))
	return int64(h.Sum64()) //nolint:gosec // the key is a hash; wrapping into the signed range is intended
}
