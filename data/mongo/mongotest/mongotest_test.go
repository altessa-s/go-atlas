// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongotest_test

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/data/mongo/mongotest"
)

// TestNewSkipsWithoutURI: with the environment variable unset the test is skipped, not failed.
func TestNewSkipsWithoutURI(t *testing.T) {
	t.Parallel()

	var skipped bool
	t.Run("gated", func(t *testing.T) {
		defer func() { skipped = t.Skipped() }()
		mongotest.New(t, mongotest.WithEnv("MONGOTEST_SURELY_UNSET_URI"))
		t.Error("New returned without a URI")
	})
	require.True(t, skipped)
}

// TestNewIT runs against the MongoDB of MONGO_URI (skipped when unset): a throwaway database with the prefix, index names,
// replica set check, and the drop on cleanup.
func TestNewIT(t *testing.T) {
	t.Parallel()

	var dbName string
	t.Run("connect", func(t *testing.T) {
		m := mongotest.New(t, mongotest.WithDatabasePrefix("mongotest_it_"))
		dbName = m.DatabaseName()
		require.Regexp(t, regexp.MustCompile(`^mongotest_it_[0-9a-f]{8}$`), dbName)

		require.Empty(t, mongotest.IndexNames(t, m, "missing"))
		coll := m.Database().Collection("things")
		_, err := coll.Indexes().CreateOne(t.Context(), mongo.IndexModel{Keys: bson.D{{Key: "b", Value: 1}}})
		require.NoError(t, err)
		_, err = coll.Indexes().CreateOne(t.Context(), mongo.IndexModel{Keys: bson.D{{Key: "a", Value: 1}}})
		require.NoError(t, err)
		require.Equal(t, []string{"_id_", "a_1", "b_1"}, mongotest.IndexNames(t, m, "things"))

		mongotest.RequireReplicaSet(t, m)
	})
	if dbName == "" {
		t.Skip("MONGO_URI not set")
	}

	// The cleanup dropped the database: a fresh connection no longer lists it.
	check := mongotest.New(t)
	names, err := check.Client().ListDatabaseNames(t.Context(), bson.D{{Key: "name", Value: dbName}})
	require.NoError(t, err)
	require.Empty(t, names)
}
