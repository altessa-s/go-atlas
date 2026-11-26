// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/drivertest"

	"github.com/altessa-s/go-atlas/store/mongo/storages/memory"

	atlas "github.com/altessa-s/go-atlas/store/mongo"
)

func TestListCursorStoredNil(t *testing.T) {
	t.Parallel()

	storage := memory.New(time.Minute)
	t.Cleanup(storage.Close)
	const token = "01HQVZX3M8RFKJH9Q5W0BXYZ12"
	require.NoError(t, storage.Store(t.Context(), token, nil))

	opts := options.Client()
	opts.Deployment = drivertest.NewMockDeployment()
	client, err := mongo.Connect(opts)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Disconnect(context.WithoutCancel(t.Context()))) })
	_, err = atlas.ListCursor[bson.M](t.Context(), client.Database("db").Collection("items"),
		atlas.WithListCursorStorage(storage), atlas.WithListCursorCursor(token))
	require.ErrorIs(t, err, atlas.ErrCursorNotFound)
}
