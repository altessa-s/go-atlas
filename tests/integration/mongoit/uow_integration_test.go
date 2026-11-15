// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongoit_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/messaging/eventbus/uow"

	datamongo "github.com/altessa-s/go-atlas/store/mongo"
)

// uowFixture connects a throwaway database with an existing collection and a uow runner over it.
func uowFixture(t *testing.T) (*datamongo.Mongo, *mongo.Collection, *uow.Runner) {
	t.Helper()
	c := client(t)
	m, err := connect(t, c, "mongoit_uow_")
	require.NoError(t, err)
	coll := c.Database(m.DatabaseName()).Collection("items")
	_, err = coll.InsertOne(t.Context(), bson.M{"_id": "seed"}) // the collection must exist before a transaction writes to it
	require.NoError(t, err)
	return m, coll, uow.New(m, nil, uow.WithDetector(datamongo.Transaction))
}

func count(t *testing.T, coll *mongo.Collection, id string) int64 {
	t.Helper()
	n, err := coll.CountDocuments(t.Context(), bson.M{"_id": id})
	require.NoError(t, err)
	return n
}

func TestUOWPanicRecoveredByMongoIsNotUnknown(t *testing.T) {
	t.Parallel()
	_, coll, r := uowFixture(t)

	var hooks int
	err := r.Run(t.Context(), func(ctx context.Context) error {
		if _, err := coll.InsertOne(ctx, bson.M{"_id": "partial"}); err != nil {
			return err
		}
		if err := r.AfterCommit(ctx, func(context.Context) { hooks++ }); err != nil {
			return err
		}
		panic("boom")
	})
	require.ErrorIs(t, err, datamongo.ErrTransactionPanicked)
	require.NotErrorIs(t, err, uow.ErrUnknownOutcome)
	require.Zero(t, hooks)
	require.Zero(t, count(t, coll, "partial"))
}

func TestUOWNestedRunJoinsAndHooksFollowTheRootCommit(t *testing.T) {
	t.Parallel()
	_, coll, r := uowFixture(t)

	var hookSaw int64 = -1
	err := r.Run(t.Context(), func(ctx context.Context) error {
		if err := r.Run(ctx, func(nctx context.Context) error {
			if _, err := coll.InsertOne(nctx, bson.M{"_id": "nested"}); err != nil {
				return err
			}
			return r.AfterCommit(nctx, func(hctx context.Context) {
				n, err := coll.CountDocuments(hctx, bson.M{"_id": "nested"})
				if err == nil {
					hookSaw = n
				}
			})
		}); err != nil {
			return err
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), hookSaw, "the hook must observe the committed write")
}

func TestUOWNestedRegistrationsDroppedOnRootAbort(t *testing.T) {
	t.Parallel()
	_, coll, r := uowFixture(t)

	errAbort := errors.New("abort")
	var hooks int
	err := r.Run(t.Context(), func(ctx context.Context) error {
		if err := r.Run(ctx, func(nctx context.Context) error {
			if _, err := coll.InsertOne(nctx, bson.M{"_id": "rolled-back"}); err != nil {
				return err
			}
			return r.AfterCommit(nctx, func(context.Context) { hooks++ })
		}); err != nil {
			return err
		}
		return errAbort
	})
	require.ErrorIs(t, err, errAbort)
	require.Zero(t, hooks)
	require.Zero(t, count(t, coll, "rolled-back"))
}

func TestUOWRejectsForeignTransaction(t *testing.T) {
	t.Parallel()
	m, coll, r := uowFixture(t)

	errRollback := errors.New("rollback")
	var runErr, hookErr error
	err := m.WithTransaction(t.Context(), func(ctx context.Context) error {
		runErr = r.Run(ctx, func(inner context.Context) error {
			_, err := coll.InsertOne(inner, bson.M{"_id": "foreign"})
			return err
		})
		hookErr = r.AfterCommit(ctx, func(context.Context) {})
		return errRollback
	})
	require.ErrorIs(t, err, errRollback)
	require.ErrorIs(t, runErr, uow.ErrForeignTransaction)
	require.ErrorIs(t, hookErr, uow.ErrForeignTransaction)
	require.Zero(t, count(t, coll, "foreign"))
}

// TestUOWIndependentInnerTransactionIsForeign: review finding R26-1 on MongoDB — inside runner A's transaction, store/mongo
// starts an independent transaction for another client; A's unit must not accept registrations made with that transaction's
// context, which may abort while A commits.
func TestUOWIndependentInnerTransactionIsForeign(t *testing.T) {
	t.Parallel()
	_, coll, r := uowFixture(t)
	other, err := connect(t, client(t), "mongoit_uow_other_")
	require.NoError(t, err)
	errAbortInner := errors.New("inner aborted")

	var hooks int
	require.NoError(t, r.Run(t.Context(), func(ctx context.Context) error {
		if _, err := coll.InsertOne(ctx, bson.M{"_id": "outer"}); err != nil {
			return err
		}
		innerErr := other.WithTransaction(ctx, func(innerCtx context.Context) error {
			require.ErrorIs(t, r.AfterCommit(innerCtx, func(context.Context) { hooks += 100 }), uow.ErrForeignTransaction)
			require.ErrorIs(t, uow.OnCommit(innerCtx, uow.Effect{Label: "inner"}), uow.ErrForeignTransaction)
			return errAbortInner
		})
		require.ErrorIs(t, innerErr, errAbortInner)
		return r.AfterCommit(ctx, func(context.Context) { hooks++ })
	}))
	require.Equal(t, 1, hooks)
	require.Equal(t, int64(1), count(t, coll, "outer"))
}
