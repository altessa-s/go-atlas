// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongoit_test

import (
	"context"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/tests/integration/internal/livebackend"

	coreconcurrency "github.com/altessa-s/go-atlas-core/runtime/concurrency"
	datamongo "github.com/altessa-s/go-atlas/store/mongo"
	mongoopts "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// client connects to the compose replica set and skips when it is not
// reachable. directConnection: the single-node set advertises its
// in-container address; see tests/integration/README.md.
func client(t *testing.T) *mongo.Client {
	t.Helper()
	c, err := mongo.Connect(mongoopts.Client().ApplyURI(livebackend.MongoURI()))
	if err != nil {
		t.Skipf("mongodb not available: %v", err)
	}
	if err = c.Ping(t.Context(), nil); err != nil {
		_ = c.Disconnect(context.WithoutCancel(t.Context()))
		t.Skipf("mongodb not reachable: %v", err)
	}
	t.Cleanup(func() { _ = c.Disconnect(context.WithoutCancel(t.Context())) })
	return c
}

// connect builds and connects a store/mongo client on a throwaway database,
// running the registered migrations, and drops the database on cleanup.
func connect(t *testing.T, c *mongo.Client, prefix string, opts ...datamongo.Option) (*datamongo.Mongo, error) {
	t.Helper()
	// Keep the database name short; a wall-clock timestamp is not unique
	// across concurrent fixtures and can make one cleanup drop another database.
	name := prefix + rand.Text()
	t.Cleanup(func() { _ = c.Database(name).Drop(context.WithoutCancel(t.Context())) })
	m, err := datamongo.New(name, append([]datamongo.Option{datamongo.WithClient(c)}, opts...)...)
	if err != nil {
		return nil, err
	}
	return m, m.Connect(t.Context())
}

func TestWithTransactionAbortsOnPanic(t *testing.T) {
	t.Parallel()
	c := client(t)
	m, err := connect(t, c, "mongoit_tx_")
	require.NoError(t, err)

	coll := c.Database(m.DatabaseName()).Collection("accounts")
	_, err = coll.InsertOne(t.Context(), bson.M{"_id": "seed"}) // the collection must exist before a transaction writes to it
	require.NoError(t, err)

	err = m.WithTransaction(t.Context(), func(ctx context.Context) error {
		if _, err := coll.InsertOne(ctx, bson.M{"_id": "partial"}); err != nil {
			return err
		}
		panic("boom")
	})
	require.ErrorIs(t, err, datamongo.ErrTransactionPanicked)

	n, err := coll.CountDocuments(t.Context(), bson.M{"_id": "partial"})
	require.NoError(t, err)
	require.Zero(t, n, "a panicking transaction must not commit its writes")
}

func TestFailedMigrationKeepsAppliedOnes(t *testing.T) {
	t.Parallel()
	c := client(t)
	m, err := connect(t, c, failingPrefix)
	require.ErrorIs(t, err, errGate)

	n, err := c.Database(m.DatabaseName()).Collection(markerCollection).CountDocuments(t.Context(), bson.M{})
	require.NoError(t, err)
	require.Equal(t, int64(1), n, "migration 1 stays applied by default")
}

func TestFailedMigrationRollsBackWhenAsked(t *testing.T) {
	t.Parallel()
	c := client(t)
	m, err := connect(t, c, failingPrefix, datamongo.WithRollbackMigrationsOnFailure())
	require.ErrorIs(t, err, errGate)

	n, err := c.Database(m.DatabaseName()).Collection(markerCollection).CountDocuments(t.Context(), bson.M{})
	require.NoError(t, err)
	require.Zero(t, n, "the full rollback undoes migration 1")
}

// TestConcurrentConnectsMigrateTheirOwnDatabase runs Connect on several
// databases at once: each must end with its own migrations applied, which a
// process-global migration database would break.
func TestConcurrentConnectsMigrateTheirOwnDatabase(t *testing.T) {
	t.Parallel()
	c := client(t)

	const clients = 6
	names, err := coreconcurrency.ProcessCollect(t.Context(), make([]int, clients), func(context.Context, int) (string, error) {
		m, err := connect(t, c, "mongoit_many_")
		if err != nil {
			return "", err
		}
		return m.DatabaseName(), nil
	}, coreconcurrency.WithConcurrency[int](clients))
	require.NoError(t, err)
	require.Len(t, names, clients)
	seen := make(map[string]struct{}, clients)

	for _, name := range names {
		require.NotContains(t, seen, name, "each concurrent connection needs its own database")
		seen[name] = struct{}{}
		n, err := c.Database(name).Collection(markerCollection).CountDocuments(t.Context(), bson.M{})
		require.NoError(t, err)
		require.Equal(t, int64(1), n, "%s ran its own migrations", name)
	}
}
