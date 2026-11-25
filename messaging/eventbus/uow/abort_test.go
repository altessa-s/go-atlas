// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package uow_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/messaging/eventbus/uow"
	"github.com/altessa-s/go-atlas/messaging/eventbus/uow/uowtest"

	datamongo "github.com/altessa-s/go-atlas/store/mongo"
	mongooptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestRun_ExplicitMongoAbort(t *testing.T) {
	t.Parallel()
	// Empty transactions exercise the driver's real state machine without server I/O.
	client, err := mongo.Connect(mongooptions.Client().ApplyURI("mongodb://localhost:27017").SetServerSelectionTimeout(time.Second))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Disconnect(context.WithoutCancel(t.Context()))) })
	session, err := client.StartSession()
	require.NoError(t, err)
	defer session.EndSession(context.WithoutCancel(t.Context()))
	committer := committerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		_, err := session.WithTransaction(ctx, func(ctx context.Context) (any, error) { return nil, fn(ctx) })
		return err
	})
	// Pin the surprising driver behavior responsible for the regression.
	require.NoError(t, committer.WithTransaction(t.Context(), func(ctx context.Context) error {
		return mongo.SessionFromContext(ctx).AbortTransaction(ctx)
	}))
	tests := []struct {
		name string
		end  func(ctx context.Context) error
	}{
		{name: "abort", end: func(ctx context.Context) error { return mongo.SessionFromContext(ctx).AbortTransaction(ctx) }},
		{name: "abort and restart", end: func(ctx context.Context) error {
			sess := mongo.SessionFromContext(ctx)
			if err := sess.AbortTransaction(ctx); err != nil {
				return err
			}
			// The driver would commit the replacement, so only its identity tells the transactions apart.
			return sess.StartTransaction()
		}},
	}
	// The subtests share the session, so they run sequentially.
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := uow.New(committer, nil, uow.WithDetector(datamongo.Transaction))
			var hooks, effects int
			var retained context.Context
			err := r.Run(t.Context(), func(ctx context.Context) error {
				retained = ctx
				require.NoError(t, r.AfterCommit(ctx, func(context.Context) { hooks++ }))
				require.NoError(t, uow.OnCommit(ctx, uow.Effect{Apply: func(context.Context) error { effects++; return nil }}))
				return r.Run(ctx, tc.end)
			})
			require.ErrorIs(t, err, uow.ErrForeignTransaction)
			require.NotErrorIs(t, err, uow.ErrUnknownOutcome)
			require.Zero(t, hooks)
			require.Zero(t, effects)
			require.ErrorIs(t, r.AfterCommit(retained, nil), uow.ErrScopeClosed)
			require.False(t, session.TransactionRunning(), "the Committer ended the attempt")
		})
	}
}

func TestRun_TransactionChangedBeforeBodyReturns(t *testing.T) {
	t.Parallel()
	for _, active := range []bool{false, true} {
		t.Run(map[bool]string{false: "ended", true: "replaced"}[active], func(t *testing.T) {
			t.Parallel()
			changed := false
			f := &uowtest.Fake{}
			r := uow.New(f, nil, uow.WithDetector(func(ctx context.Context) (any, bool) {
				tx, ok := uowtest.Transaction(ctx)
				if changed && ok {
					return "replacement", active
				}
				return tx, ok
			}))
			var ran int
			err := r.Run(t.Context(), func(ctx context.Context) error {
				require.NoError(t, r.AfterCommit(ctx, func(context.Context) { ran++ }))
				require.NoError(t, uow.OnCommit(ctx, uow.Effect{Apply: func(context.Context) error { ran++; return nil }}))
				changed = true
				return nil
			})
			require.ErrorIs(t, err, uow.ErrForeignTransaction)
			require.NotErrorIs(t, err, uow.ErrUnknownOutcome)
			require.Zero(t, ran)
		})
	}
}

func TestRun_RejectsMongoTransactionByDefault(t *testing.T) {
	t.Parallel()
	// An empty transaction exercises the driver without server I/O.
	client, err := mongo.Connect(mongooptions.Client().ApplyURI("mongodb://localhost:27017"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Disconnect(context.WithoutCancel(t.Context()))) })
	session, err := client.StartSession()
	require.NoError(t, err)
	defer session.EndSession(context.WithoutCancel(t.Context()))
	require.NoError(t, session.StartTransaction())

	// Detection must happen before the committer attempts to start a session.
	r := uow.New(&datamongo.Mongo{}, nil, uow.WithRunHooksOutsideTransaction())
	ctx := mongo.NewSessionContext(t.Context(), session)
	require.ErrorIs(t, r.Run(ctx, func(context.Context) error {
		t.Error("body called inside another MongoDB transaction")
		return nil
	}), uow.ErrForeignTransaction)
	require.ErrorIs(t, r.AfterCommit(ctx, func(context.Context) {
		t.Error("hook ran inside another MongoDB transaction")
	}), uow.ErrForeignTransaction)
	require.True(t, session.TransactionRunning())
}

func TestRun_RejectsFakeTransactionByDefault(t *testing.T) {
	t.Parallel()
	f := new(uowtest.Fake)
	r := uow.New(f, nil)
	require.ErrorIs(t, f.WithTransaction(t.Context(), func(ctx context.Context) error {
		return r.Run(ctx, func(context.Context) error {
			t.Error("runner entered a transaction it does not own")
			return nil
		})
	}), uow.ErrForeignTransaction)
}
