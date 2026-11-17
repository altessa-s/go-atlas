// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxit_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/tests/integration/outboxit"

	outboxsql "github.com/altessa-s/go-atlas/reliability/outbox/storages/sqldb"
)

// newNotifySQLFixture is a PostgreSQL fixture whose store signals on save and
// watches through the pgx listener, on the channel derived from its table.
func newNotifySQLFixture(tb testing.TB) *fixture {
	tb.Helper()

	spec := sqlSpecs()[0]
	require.Equal(tb, outboxsql.DialectPostgres, spec.dialect)
	b := newSQLFixtureWithoutSchema(tb, spec)
	store, err := outboxsql.New(b.db, spec.dialect, outboxsql.WithTableName(b.events),
		outboxsql.WithNotify(), outboxsql.WithListener(outboxit.PGListener{DSN: spec.DSN()}))
	require.NoError(tb, err)
	require.NoError(tb, store.EnsureSchema(tb.Context()))

	return &fixture{store: store, recorder: outboxit.NewRecorder(), sql: b}
}

// The notification alone carries a saved event through to its handler, with no
// schedule and no manual cycle.
func TestWatch_SQLNotifyDeliversWithoutAPollCycle(t *testing.T) {
	t.Parallel()

	f := newNotifySQLFixture(t)
	ob := f.newOutbox(t)
	f.startWatcher(t, ob)
	f.awaitWatcher(t, ob)

	saved := f.save(t, ob, event("billing.invoice.paid", `{"invoice":1}`))

	require.Eventually(t, func() bool { return f.recorder.Count() == 1 }, settleWindow, samplingInterval,
		"the notification never woke the dispatcher:\n%s", f.recorder)
	require.Equal(t, []string{`{"invoice":1}`}, f.recorder.Payloads())
	f.awaitSent(t, saved[0].Id)
}

// pg_notify runs in the saving transaction, so the listener hears of an event
// at commit and never of one whose transaction rolled back.
func TestWatch_SQLNotifyDeliversNothingUntilTheTransactionCommits(t *testing.T) {
	t.Parallel()

	f := newNotifySQLFixture(t)
	ob := f.newOutbox(t)
	f.startWatcher(t, ob)
	f.awaitWatcher(t, ob)

	errAbort := errors.New("abort")
	err := f.tryTransaction(t, func(txCtx context.Context) error {
		require.NoError(t, ob.Save(txCtx, event("billing.order.aborted", `{"order":"o-0"}`)))
		return errAbort
	})
	require.ErrorIs(t, err, errAbort)

	f.inTransaction(t, func(txCtx context.Context) error {
		if err := ob.Save(txCtx, event("billing.order.created", `{"order":"o-1"}`)); err != nil {
			return err
		}
		require.False(t, settled(func() bool { return f.recorder.Count() > 0 }, uncommittedWindow),
			"an uncommitted event must not be dispatched:\n%s", f.recorder.Timeline())
		return nil
	})

	require.Eventually(t, func() bool { return f.recorder.Count() == 1 }, settleWindow, samplingInterval,
		"the commit never produced a notification:\n%s", f.recorder)
	require.Equal(t, []string{`{"order":"o-1"}`}, f.recorder.Payloads())
}
