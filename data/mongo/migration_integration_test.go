// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/data/mongo/mongotest"

	datamongo "github.com/altessa-s/go-atlas/data/mongo"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Every Connect of this test binary runs the typed migration below; it writes only on databases with typedMigrationPrefix and is a
// recorded no-op elsewhere.
const (
	typedMigrationVersion     = 9_000_001
	typedMigrationDescription = "typed_marker"
	typedMigrationPrefix      = "migtyped_it_"
)

func init() {
	datamongo.MustRegisterMigration(typedMigrationVersion, typedMigrationDescription, func(ctx context.Context, db *mongo.Database) error {
		if !strings.HasPrefix(db.Name(), typedMigrationPrefix) {
			return nil
		}
		_, err := db.Collection("markers").InsertOne(ctx, bson.M{"_id": "typed"})
		return err
	}, nil)
}

// markerCount counts the marker documents the typed migration wrote.
func markerCount(t *testing.T, db *mongo.Database) int64 {
	t.Helper()
	n, err := db.Collection("markers").CountDocuments(t.Context(), bson.M{"_id": "typed"})
	require.NoError(t, err)
	return n
}

// TestRegisterMigrationAppliesIT: Connect applies a typed migration once and records it in the mongo-migrate format.
func TestRegisterMigrationAppliesIT(t *testing.T) {
	t.Parallel()
	m := mongotest.New(t, mongotest.WithDatabasePrefix(typedMigrationPrefix))
	require.EqualValues(t, 1, markerCount(t, m.Database()))

	var rec struct {
		Version     uint64    `bson:"version"`
		Description string    `bson:"description"`
		Timestamp   time.Time `bson:"timestamp"`
	}
	require.NoError(t, m.Database().Collection(datamongo.MigrationCollectionName).
		FindOne(t.Context(), bson.M{"version": typedMigrationVersion}).Decode(&rec))
	require.Equal(t, typedMigrationDescription, rec.Description)
	require.False(t, rec.Timestamp.IsZero())

	// A second client of the same database finds nothing to apply.
	again, err := datamongo.New(m.DatabaseName(), datamongo.WithClient(m.Client()))
	require.NoError(t, err)
	require.NoError(t, again.Connect(t.Context()))
	require.EqualValues(t, 1, markerCount(t, m.Database()))
}

// TestRegisterMigrationSkipsAppliedVersionIT: a version already recorded (applied earlier through mongo-migrate under the same number)
// never runs again.
func TestRegisterMigrationSkipsAppliedVersionIT(t *testing.T) {
	t.Parallel()
	uri := os.Getenv(mongotest.DefaultEnv)
	if uri == "" {
		t.Skipf("%s not set; skipping integration test", mongotest.DefaultEnv)
	}
	suffix := make([]byte, 4)
	_, _ = rand.Read(suffix)
	dbName := typedMigrationPrefix + "applied_" + hex.EncodeToString(suffix)

	client, err := mongo.Connect(mongoOptions.Client().ApplyURI(uri))
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 30*time.Second)
		defer cancel()
		_ = client.Database(dbName).Drop(ctx)
		_ = client.Disconnect(ctx)
	})
	_, err = client.Database(dbName).Collection(datamongo.MigrationCollectionName).InsertOne(t.Context(), bson.M{
		"version": uint64(typedMigrationVersion), "description": "applied_by_mongo_migrate", "timestamp": time.Now().UTC(),
	})
	require.NoError(t, err)

	m, err := datamongo.New(dbName, datamongo.WithClient(client))
	require.NoError(t, err)
	require.NoError(t, m.Connect(t.Context()))
	require.Zero(t, markerCount(t, client.Database(dbName)), "the recorded version is not applied again")
}
