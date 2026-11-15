// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"

	datamongo "github.com/altessa-s/go-atlas/store/mongo"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// TestTransactionIdentity runs empty transactions, which exercise the driver's session state machine without server I/O. A later
// transaction of the same session must not be reported as the earlier one; this also pins the driver layout txnNumber reads.
func TestTransactionIdentity(t *testing.T) {
	t.Parallel()
	client, err := mongo.Connect(mongoOptions.Client().ApplyURI("mongodb://localhost:27017").SetServerSelectionTimeout(time.Second))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Disconnect(context.WithoutCancel(t.Context()))) })
	sess, err := client.StartSession()
	require.NoError(t, err)
	defer sess.EndSession(context.WithoutCancel(t.Context()))
	ctx := mongo.NewSessionContext(t.Context(), sess)

	_, ok := datamongo.Transaction(ctx)
	require.False(t, ok, "a session without a transaction")

	require.NoError(t, sess.StartTransaction())
	first, ok := datamongo.Transaction(ctx)
	require.True(t, ok)
	again, ok := datamongo.Transaction(mongo.NewSessionContext(t.Context(), sess))
	require.True(t, ok)
	require.Equal(t, first, again, "every context of one transaction reports the same identity")

	require.NoError(t, sess.AbortTransaction(ctx))
	_, ok = datamongo.Transaction(ctx)
	require.False(t, ok, "aborted")

	require.NoError(t, sess.StartTransaction())
	second, ok := datamongo.Transaction(ctx)
	require.True(t, ok)
	require.NotEqual(t, first, second, "a restarted transaction of the same session is another transaction")

	other, err := client.StartSession()
	require.NoError(t, err)
	defer other.EndSession(context.WithoutCancel(t.Context()))
	require.NoError(t, other.StartTransaction())
	foreign, ok := datamongo.Transaction(mongo.NewSessionContext(t.Context(), other))
	require.True(t, ok)
	require.NotEqual(t, second, foreign, "another session's transaction")
	require.NoError(t, other.AbortTransaction(t.Context()))
	require.NoError(t, sess.AbortTransaction(ctx))
}
