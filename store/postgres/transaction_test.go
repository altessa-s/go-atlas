// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/reliability/outbox"

	outboxsql "github.com/altessa-s/go-atlas/reliability/outbox/storages/sqldb"
)

func idRows(string, []any) testhelpers.FakeSQLReply {
	return testhelpers.FakeSQLReply{Columns: []string{"id"}}
}

func TestQuerierUsesAttachedTransaction(t *testing.T) {
	t.Parallel()

	db, fake := testhelpers.NewFakeSQL(t, idRows)
	querier := Querier(db)

	read := func(ctx context.Context) error {
		rows, err := querier.QueryContext(ctx, "SELECT id FROM inventory")
		if err != nil {
			return err
		}

		if err = rows.Close(); err != nil {
			return err
		}

		_, err = querier.ExecContext(ctx, "DELETE FROM inventory")

		return err
	}

	if err := read(t.Context()); err != nil {
		t.Fatal(err)
	}

	if err := WithTransaction(t.Context(), db, read); err != nil {
		t.Fatal(err)
	}

	calls := fake.Calls()
	if len(calls) != 4 || calls[0].InTx || calls[1].InTx || !calls[2].InTx || !calls[3].InTx {
		t.Fatalf("calls=%v", calls)
	}

	if fake.RowsClosed() != 2 || fake.Commits() != 1 {
		t.Fatalf("closed=%d commits=%d", fake.RowsClosed(), fake.Commits())
	}
}

func TestNestedTransactionsShareOwnerTransaction(t *testing.T) {
	t.Parallel()

	db, fake := testhelpers.NewFakeSQL(t, nil)

	events, err := outboxsql.New(
		db,
		outboxsql.DialectPostgres,
		outboxsql.WithTableName("app.events_outbox"),
	)
	if err != nil {
		t.Fatal(err)
	}

	err = WithTransaction(t.Context(), db, func(outer context.Context) error {
		owner, ok, _ := Tx(outer)
		if !ok {
			return errors.New("transaction missing from context")
		}

		if err := events.SaveEvents(outboxsql.WithTx(outer, owner), outbox.Event{Id: "event", Key: "inventory.updated"}); err != nil {
			return err
		}

		return WithSnapshot(outer, db, func(inner context.Context) error {
			if nested, _, _ := Tx(inner); nested != owner {
				return errors.New("nested call opened another transaction")
			}

			_, err := Queryer(inner, db).ExecContext(inner, "UPDATE inventory SET n = 1")

			return err
		})
	})
	if err != nil {
		t.Fatal(err)
	}

	calls := fake.Calls()
	if len(calls) != 2 || !calls[0].InTx || !calls[1].InTx {
		t.Fatalf("outbox and storage writes left the transaction: %v", calls)
	}

	if fake.Commits() != 1 || fake.Rollbacks() != 0 {
		t.Fatalf("commits=%d rollbacks=%d", fake.Commits(), fake.Rollbacks())
	}

	if _, ok, _ := Tx(t.Context()); ok {
		t.Fatal("transaction leaked outside its handler")
	}
}

func TestTransactionRollsBackOnHandlerError(t *testing.T) {
	t.Parallel()

	db, fake := testhelpers.NewFakeSQL(t, nil)
	cause := errors.New("handler failed")

	err := WithTransaction(t.Context(), db, func(ctx context.Context) error {
		if _, err := Queryer(ctx, db).ExecContext(ctx, "UPDATE inventory SET n = 1"); err != nil {
			return err
		}

		return cause
	})
	if !errors.Is(err, cause) {
		t.Fatalf("error=%v", err)
	}

	if fake.Commits() != 0 || fake.Rollbacks() != 1 {
		t.Fatalf("commits=%d rollbacks=%d", fake.Commits(), fake.Rollbacks())
	}
}

func TestTransactionRejectsAnotherDatabase(t *testing.T) {
	t.Parallel()

	first, _ := testhelpers.NewFakeSQL(t, nil)
	second, _ := testhelpers.NewFakeSQL(t, nil)

	err := WithTransaction(t.Context(), first, func(ctx context.Context) error {
		return WithTransaction(ctx, second, func(context.Context) error { return nil })
	})
	if err == nil {
		t.Fatal("transaction of another database reused")
	}
}

func TestQueryerOutsideTransactionUsesPool(t *testing.T) {
	t.Parallel()

	db, _ := testhelpers.NewFakeSQL(t, nil)
	if _, err := Queryer(t.Context(), db).ExecContext(t.Context(), "SELECT 1"); err != nil {
		t.Fatal(err)
	}
}
