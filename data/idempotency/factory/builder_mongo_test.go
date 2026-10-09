// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/data/idempotency/factory"

	idempotencyconfig "github.com/altessa-s/go-atlas/config/idempotency"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

func mongoConfig() *idempotencyconfig.Config {
	return &idempotencyconfig.Config{
		TTL:             time.Hour,
		MaxLockDuration: time.Minute,
		Storage: &idempotencyconfig.StorageConfig{
			Type:  idempotencyconfig.StorageTypeMongo,
			Mongo: &idempotencyconfig.MongoStorageConfig{Collection: "idem"},
		},
	}
}

func TestBuild_MongoRequiresDatabase(t *testing.T) {
	t.Parallel()

	_, err := factory.New(mongoConfig()).Build()
	require.Error(t, err)
	require.Contains(t, err.Error(), "mongo database")
}

func TestBuild_Mongo(t *testing.T) {
	t.Parallel()

	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		uri = "mongodb://localhost:27017"
	}
	client, err := mongo.Connect(mongoOptions.Client().ApplyURI(uri).SetTimeout(10 * time.Second))
	if err != nil {
		t.Skipf("mongodb not available: %v", err)
	}
	if err := client.Ping(t.Context(), nil); err != nil {
		_ = client.Disconnect(context.WithoutCancel(t.Context()))
		t.Skipf("mongodb not reachable: %v", err)
	}
	if _, err := client.ListDatabaseNames(t.Context(), bson.D{}); err != nil {
		_ = client.Disconnect(context.WithoutCancel(t.Context()))
		t.Skipf("mongodb not usable for tests: %v", err)
	}
	db := client.Database("idempotency_factory_" + strconv.FormatInt(time.Now().UnixNano(), 36))
	t.Cleanup(func() {
		_ = db.Drop(context.WithoutCancel(t.Context()))
		_ = client.Disconnect(context.WithoutCancel(t.Context()))
	})

	keeper, err := factory.New(mongoConfig()).UseMongoDatabase(db).Build()
	require.NoError(t, err)
	ok, held, err := keeper.AttemptLock(t.Context(), "k")
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, keeper.Complete(t.Context(), "k", nil, held))

	n, err := db.Collection("idem").CountDocuments(t.Context(), bson.D{})
	require.NoError(t, err)
	require.Equal(t, int64(1), n, "the configured collection holds the key")
}
