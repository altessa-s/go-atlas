// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package migrations

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/drivertest"

	mongooptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestRunCleansOnlyFailedStep(t *testing.T) {
	t.Parallel()

	upErr := errors.New("up failed")
	cleanupErr := errors.New("cleanup failed")
	for _, tc := range []struct {
		name       string
		cleanup    bool
		cleanupErr error
		rollback   bool
		leaseLost  bool
		wantCalls  []string
	}{
		{name: "without handler", wantCalls: []string{"first up", "second up"}},
		{name: "without rollback", cleanup: true, wantCalls: []string{"first up", "second up", "second cleanup"}},
		{name: "before rollback", cleanup: true, cleanupErr: cleanupErr, rollback: true,
			wantCalls: []string{"first up", "second up", "second cleanup", "first down"}},
		{name: "lease lost", cleanup: true, leaseLost: true, wantCalls: []string{"first up", "second up"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ok := bson.D{{Key: "ok", Value: 1}, {Key: "n", Value: 1}}
			collections := migrationBatch("db.$cmd.listCollections", bson.D{{Key: "name", Value: MigrationCollectionName}})
			replies := []bson.D{
				ok, ok, ok, // Seed, acquire, and confirm the lease.
				collections, migrationBatch("db.migrations"), // Read the initial ledger version.
				ok, // Record the first step.
			}
			switch {
			case tc.leaseLost:
				replies = append(replies, bson.D{{Key: "ok", Value: 1}, {Key: "n", Value: 0}})
			case tc.cleanup:
				replies = append(replies, ok) // Verify the lease for cleanup.
			}
			if tc.rollback {
				replies = append(replies, ok, collections, migrationBatch("db.migrations", bson.D{{Key: "version", Value: 1}}), ok)
			}
			replies = append(replies, ok) // Release the lease.
			var recorded []int64
			clientOpts := mongooptions.Client().SetMonitor(&event.CommandMonitor{
				Started: func(_ context.Context, e *event.CommandStartedEvent) {
					if e.CommandName == "insert" && e.Command.Lookup("insert").StringValue() == MigrationCollectionName {
						doc := e.Command.Lookup("documents").Array().Index(0).Document()
						recorded = append(recorded, doc.Lookup("version").AsInt64())
					}
				},
			})
			clientOpts.Deployment = drivertest.NewMockDeployment(replies...)
			client, err := mongo.Connect(clientOpts)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, client.Disconnect(context.WithoutCancel(t.Context()))) })
			db := client.Database("db")
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			var registry Registry
			var calls []string
			record := func(name string, err error) MigrationFunc {
				return func(context.Context, *mongo.Database) error {
					calls = append(calls, name)
					return err
				}
			}
			require.NoError(t, registry.Register(1, "first", record("first up", nil), record("first down", nil),
				WithCleanupOnFailure(record("first cleanup", nil))))
			secondUp := func(context.Context, *mongo.Database) error {
				cancel() // Cleanup and rollback need a fresh budget on the lease scope.
				calls = append(calls, "second up")
				return upErr
			}
			var migrationOpts []MigrationOption
			if tc.cleanup {
				migrationOpts = []MigrationOption{WithCleanupOnFailure(func(ctx context.Context, gotDB *mongo.Database) error {
					require.Same(t, db, gotDB)
					require.NoError(t, ctx.Err())
					_, bounded := ctx.Deadline()
					require.True(t, bounded)
					calls = append(calls, "second cleanup")
					return tc.cleanupErr
				})}
			}
			require.NoError(t, registry.Register(2, "second", secondUp, record("second down", nil), migrationOpts...))
			require.NoError(t, registry.Register(3, "third", record("third up", nil), record("third down", nil),
				WithCleanupOnFailure(record("third cleanup", nil))))
			runOpts := []Option{WithRegistry(&registry)}
			if tc.rollback {
				runOpts = []Option{WithRegistry(&registry), WithRollbackOnFailure()}
			}
			err = Run(ctx, db, runOpts...)
			require.ErrorIs(t, err, upErr)
			require.Equal(t, tc.wantCalls, calls)
			if tc.rollback {
				require.Equal(t, []int64{1, 0}, recorded)
			} else {
				require.Equal(t, []int64{1}, recorded)
			}
			if tc.cleanupErr != nil {
				require.ErrorIs(t, err, tc.cleanupErr)
			}
			if tc.leaseLost {
				require.ErrorIs(t, err, ErrLeaseLost)
			}
		})
	}
}

func migrationBatch(namespace string, docs ...bson.D) bson.D {
	batch := make(bson.A, 0, len(docs))
	for _, doc := range docs {
		batch = append(batch, doc)
	}
	return bson.D{
		{Key: "ok", Value: 1},
		{Key: "cursor", Value: bson.D{{Key: "id", Value: int64(0)}, {Key: "ns", Value: namespace}, {Key: "firstBatch", Value: batch}}},
	}
}
