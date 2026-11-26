// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongoit_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/store/mongo/migrations"
	"github.com/altessa-s/go-atlas/tests/integration/internal/livebackend"

	mongooptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

var errStepFailed = errors.New("step failed")

// counted returns an up/down pair that counts its calls in the database, so effects survive across Run calls.
func counted(name string, ups, downs *atomic.Int32) (up, down migrations.MigrationFunc) {
	up = func(ctx context.Context, db *mongo.Database) error {
		ups.Add(1)
		_, err := db.Collection("effects").InsertOne(ctx, bson.M{"_id": name})
		return err
	}
	down = func(ctx context.Context, db *mongo.Database) error {
		downs.Add(1)
		_, err := db.Collection("effects").DeleteOne(ctx, bson.M{"_id": name})
		return err
	}
	return up, down
}

func failing(_ context.Context, _ *mongo.Database) error { return errStepFailed }

func ledgerVersion(t *testing.T, db *mongo.Database) uint64 {
	t.Helper()
	var rec struct {
		Version uint64 `bson:"version"`
	}
	err := db.Collection(migrations.MigrationCollectionName).FindOne(t.Context(), bson.D{},
		mongooptions.FindOne().SetSort(bson.D{{Key: "_id", Value: -1}})).Decode(&rec)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return 0
	}
	require.NoError(t, err)
	return rec.Version
}

func effectCount(t *testing.T, db *mongo.Database) int64 {
	t.Helper()
	n, err := db.Collection("effects").CountDocuments(t.Context(), bson.D{})
	require.NoError(t, err)
	return n
}

// TestRollbackKeepsEarlierRuns: v1 applied by an earlier run, v2 fails with rollback enabled: v1 effects and ledger version remain
// and v1's down is not called.
func TestRollbackKeepsEarlierRuns(t *testing.T) {
	t.Parallel()
	db := livebackend.MongoDatabase(t, "mig_rollback_earlier")
	var ups, downs atomic.Int32
	up1, down1 := counted("v1", &ups, &downs)

	var first migrations.Registry
	require.NoError(t, first.Register(1, "one", up1, down1))
	require.NoError(t, migrations.Run(t.Context(), db, migrations.WithRegistry(&first)))

	var second migrations.Registry
	require.NoError(t, second.Register(1, "one", up1, down1))
	require.NoError(t, second.Register(2, "two", failing, nil))
	err := migrations.Run(t.Context(), db, migrations.WithRegistry(&second), migrations.WithRollbackOnFailure())
	require.ErrorIs(t, err, errStepFailed)

	require.Equal(t, int32(1), ups.Load())
	require.Zero(t, downs.Load(), "a step of an earlier run is never reverted")
	require.Equal(t, uint64(1), ledgerVersion(t, db))
	require.Equal(t, int64(1), effectCount(t, db))
}

// TestRollbackNilDownAppliedOnce: a migration without a down is applied exactly once across two runs.
func TestRollbackNilDownAppliedOnce(t *testing.T) {
	t.Parallel()
	db := livebackend.MongoDatabase(t, "mig_rollback_nildown")
	var ups, downs atomic.Int32
	up1, _ := counted("v1", &ups, &downs)

	var reg migrations.Registry
	require.NoError(t, reg.Register(1, "one", up1, nil))
	require.NoError(t, reg.Register(2, "two", failing, nil))
	for range 2 {
		err := migrations.Run(t.Context(), db, migrations.WithRegistry(&reg), migrations.WithRollbackOnFailure())
		require.ErrorIs(t, err, errStepFailed)
	}
	require.Equal(t, int32(1), ups.Load(), "the irreversible step is not replayed")
	require.Equal(t, uint64(1), ledgerVersion(t, db))
}

// TestRollbackOnlyThisRunNewestFirst: v1 and v2 are applied in this run and v3 fails: v2 then v1 are reverted, v3's down is not called.
func TestRollbackOnlyThisRunNewestFirst(t *testing.T) {
	t.Parallel()
	db := livebackend.MongoDatabase(t, "mig_rollback_run")
	var order []string
	var ups, downs atomic.Int32
	up1, down1 := counted("v1", &ups, &downs)
	up2, down2 := counted("v2", &ups, &downs)
	var thirdDown atomic.Int32

	var reg migrations.Registry
	require.NoError(t, reg.Register(1, "one", up1, func(ctx context.Context, d *mongo.Database) error {
		order = append(order, "v1")
		return down1(ctx, d)
	}))
	require.NoError(t, reg.Register(2, "two", up2, func(ctx context.Context, d *mongo.Database) error {
		order = append(order, "v2")
		return down2(ctx, d)
	}))
	require.NoError(t, reg.Register(3, "three", failing, func(context.Context, *mongo.Database) error {
		thirdDown.Add(1)
		return nil
	}))
	err := migrations.Run(t.Context(), db, migrations.WithRegistry(&reg), migrations.WithRollbackOnFailure())
	require.ErrorIs(t, err, errStepFailed)
	require.Equal(t, []string{"v2", "v1"}, order)
	require.Zero(t, thirdDown.Load(), "the failed step's own down is not invoked")
	require.Zero(t, effectCount(t, db))
	require.Zero(t, ledgerVersion(t, db))
}

// TestRollbackStopsAtIrreversible: v1 has no down, v2 is applied, v3 fails: v2 is reverted, rollback stops at v1.
func TestRollbackStopsAtIrreversible(t *testing.T) {
	t.Parallel()
	db := livebackend.MongoDatabase(t, "mig_rollback_irrev")
	var ups, downs atomic.Int32
	up1, _ := counted("v1", &ups, &downs)
	up2, down2 := counted("v2", &ups, &downs)

	var reg migrations.Registry
	require.NoError(t, reg.Register(1, "one", up1, nil))
	require.NoError(t, reg.Register(2, "two", up2, down2))
	require.NoError(t, reg.Register(3, "three", failing, nil))
	err := migrations.Run(t.Context(), db, migrations.WithRegistry(&reg), migrations.WithRollbackOnFailure())
	require.ErrorIs(t, err, errStepFailed)
	require.ErrorIs(t, err, migrations.ErrIrreversibleMigration)
	require.Equal(t, int32(1), downs.Load())
	require.Equal(t, uint64(1), ledgerVersion(t, db))
	require.Equal(t, int64(1), effectCount(t, db))
}

// TestLeaseRenewedWhileUpIgnoresContext: A's up outlives one lease period ignoring its context while B starts meanwhile; the lease
// is renewed several times, so up is entered once.
func TestLeaseRenewedWhileUpIgnoresContext(t *testing.T) {
	t.Parallel()
	db := livebackend.MongoDatabase(t, "mig_lease_renew")
	var entered atomic.Int32
	var reg migrations.Registry
	require.NoError(t, reg.Register(1, "slow", func(context.Context, *mongo.Database) error {
		entered.Add(1)
		time.Sleep(migrations.MigrationTimeout + 5*time.Second)
		return nil
	}, nil))

	done := make(chan error, 1)
	go func() { done <- migrations.Run(t.Context(), db, migrations.WithRegistry(&reg)) }()
	time.Sleep(time.Second)
	bDone := make(chan error, 1)
	go func() { bDone <- migrations.Run(t.Context(), db, migrations.WithRegistry(&reg)) }()

	require.NoError(t, <-done)
	<-bDone // B either gave up waiting or found the version applied; it must not have run the step.
	require.Equal(t, int32(1), entered.Load())
}

// TestLeaseLostCancelsWorkAndSkipsRollback: the lock document disappears during up: the work context is canceled and no rollback runs.
func TestLeaseLostCancelsWorkAndSkipsRollback(t *testing.T) {
	t.Parallel()
	db := livebackend.MongoDatabase(t, "mig_lease_lost")
	var ups, downs atomic.Int32
	up1, down1 := counted("v1", &ups, &downs)
	var reg migrations.Registry
	require.NoError(t, reg.Register(1, "one", up1, down1))
	require.NoError(t, reg.Register(2, "two", func(ctx context.Context, d *mongo.Database) error {
		_, err := d.Collection("migrations_lock").DeleteOne(ctx, bson.M{"_id": migrations.LockID})
		require.NoError(t, err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(migrations.MigrationTimeout):
			return errors.New("work context was not canceled")
		}
	}, nil))
	err := migrations.Run(t.Context(), db, migrations.WithRegistry(&reg), migrations.WithRollbackOnFailure())
	require.ErrorIs(t, err, migrations.ErrLeaseLost)
	require.Zero(t, downs.Load(), "no rollback after ownership is lost")
	require.Equal(t, uint64(1), ledgerVersion(t, db))
}
