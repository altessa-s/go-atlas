// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongoit_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
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
	db := livebackend.MongoDatabase(t, "mg")
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
	db := livebackend.MongoDatabase(t, "mg")
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

// TestRollbackOnlyThisRunNewestFirst: v1 is applied by an earlier run; this run applies v2 and v3 and v4 fails: v3 then v2 are
// reverted, v1 and the failed step's down are not called.
func TestRollbackOnlyThisRunNewestFirst(t *testing.T) {
	t.Parallel()
	db := livebackend.MongoDatabase(t, "mg")
	var mu sync.Mutex
	var order []string
	var ups, downs atomic.Int32
	record := func(name string, down migrations.MigrationFunc) migrations.MigrationFunc {
		return func(ctx context.Context, d *mongo.Database) error {
			mu.Lock()
			order = append(order, name)
			mu.Unlock()
			return down(ctx, d)
		}
	}
	up1, down1 := counted("v1", &ups, &downs)
	up2, down2 := counted("v2", &ups, &downs)
	up3, down3 := counted("v3", &ups, &downs)
	var fourthDown atomic.Int32

	var earlier migrations.Registry
	require.NoError(t, earlier.Register(1, "one", up1, record("v1", down1)))
	require.NoError(t, migrations.Run(t.Context(), db, migrations.WithRegistry(&earlier)))

	var reg migrations.Registry
	require.NoError(t, reg.Register(1, "one", up1, record("v1", down1)))
	require.NoError(t, reg.Register(2, "two", up2, record("v2", down2)))
	require.NoError(t, reg.Register(3, "three", up3, record("v3", down3)))
	require.NoError(t, reg.Register(4, "four", failing, func(context.Context, *mongo.Database) error {
		fourthDown.Add(1)
		return nil
	}))
	err := migrations.Run(t.Context(), db, migrations.WithRegistry(&reg), migrations.WithRollbackOnFailure())
	require.ErrorIs(t, err, errStepFailed)
	require.Equal(t, []string{"v3", "v2"}, order)
	require.Zero(t, fourthDown.Load(), "the failed step's own down is not invoked")
	require.Equal(t, int64(1), effectCount(t, db), "only v1 of the earlier run remains")
	require.Equal(t, uint64(1), ledgerVersion(t, db))
}

// TestRollbackStopsAtIrreversible: v1 has no down, v2 is applied, v3 fails: v2 is reverted, rollback stops at v1.
func TestRollbackStopsAtIrreversible(t *testing.T) {
	t.Parallel()
	db := livebackend.MongoDatabase(t, "mg")
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

// lockDocument removes the lease document, as a successor taking over after expiry would replace it.
func removeLease(ctx context.Context, d *mongo.Database) error {
	_, err := d.Collection("migrations_lock").DeleteOne(ctx, bson.M{"_id": migrations.LockID})
	return err
}

// TestLeaseLostBeforeRollbackSkipsRollback: the lease is gone when the failed step returns: no down runs and the ledger is untouched.
func TestLeaseLostBeforeRollbackSkipsRollback(t *testing.T) {
	t.Parallel()
	db := livebackend.MongoDatabase(t, "mg")
	var ups, downs atomic.Int32
	up1, down1 := counted("v1", &ups, &downs)
	var reg migrations.Registry
	require.NoError(t, reg.Register(1, "one", up1, down1))
	require.NoError(t, reg.Register(2, "two", func(ctx context.Context, d *mongo.Database) error {
		require.NoError(t, removeLease(ctx, d))
		return errStepFailed
	}, nil))
	err := migrations.Run(t.Context(), db, migrations.WithRegistry(&reg), migrations.WithRollbackOnFailure())
	require.ErrorIs(t, err, errStepFailed)
	require.ErrorIs(t, err, migrations.ErrLeaseLost)
	require.Zero(t, downs.Load(), "no rollback after ownership is lost")
	require.Equal(t, uint64(1), ledgerVersion(t, db))
}

// TestLeaseLostDuringDownStopsRollback: the lease disappears while a down runs: its context is canceled, no later down runs and no
// ledger record is written.
func TestLeaseLostDuringDownStopsRollback(t *testing.T) {
	t.Parallel()
	db := livebackend.MongoDatabase(t, "mg")
	var ups, downs atomic.Int32
	up1, down1 := counted("v1", &ups, &downs)
	up2, _ := counted("v2", &ups, &downs)
	var reg migrations.Registry
	require.NoError(t, reg.Register(1, "one", up1, down1))
	require.NoError(t, reg.Register(2, "two", up2, func(ctx context.Context, d *mongo.Database) error {
		downs.Add(1)
		require.NoError(t, removeLease(ctx, d))
		<-ctx.Done() // Canceled by the heartbeat once the renewal finds the lease gone.
		return ctx.Err()
	}))
	require.NoError(t, reg.Register(3, "three", failing, nil))
	err := migrations.Run(t.Context(), db, migrations.WithRegistry(&reg), migrations.WithRollbackOnFailure())
	require.ErrorIs(t, err, errStepFailed)
	require.ErrorIs(t, err, migrations.ErrLeaseLost)
	require.Equal(t, int32(1), downs.Load(), "only the interrupted down of v2 ran")
	require.Equal(t, uint64(2), ledgerVersion(t, db), "no ledger write after the loss")
}

// TestRollbackStopsOnLedgerDrift: the ledger holds a version this run did not record: nothing is reverted.
func TestRollbackStopsOnLedgerDrift(t *testing.T) {
	t.Parallel()
	db := livebackend.MongoDatabase(t, "mg")
	var ups, downs atomic.Int32
	up1, down1 := counted("v1", &ups, &downs)
	var reg migrations.Registry
	require.NoError(t, reg.Register(1, "one", up1, down1))
	require.NoError(t, reg.Register(2, "two", func(ctx context.Context, d *mongo.Database) error {
		_, err := d.Collection(migrations.MigrationCollectionName).InsertOne(ctx, bson.M{"version": int64(99), "timestamp": time.Now()})
		require.NoError(t, err)
		return errStepFailed
	}, nil))
	err := migrations.Run(t.Context(), db, migrations.WithRegistry(&reg), migrations.WithRollbackOnFailure())
	require.ErrorIs(t, err, errStepFailed)
	require.Zero(t, downs.Load())
	require.Equal(t, uint64(99), ledgerVersion(t, db))
}

// leaseDocument reads the lease expiry, the lease's observable heartbeat.
func leaseExpiry(ctx context.Context, d *mongo.Database) (time.Time, error) {
	var doc struct {
		ExpiresAt time.Time `bson:"expiresAt"`
	}
	err := d.Collection("migrations_lock").FindOne(ctx, bson.M{"_id": migrations.LockID}).Decode(&doc)
	return doc.ExpiresAt, err
}

// TestLeaseHeldPastTheBudget: A's step ignores its deadline-bound context past the work budget while B waits for the lease. The lease
// is renewed throughout, so the step is entered once and never overlaps; the step's result is recorded although it finished after the
// budget, and a later run applies nothing.
func TestLeaseHeldPastTheBudget(t *testing.T) {
	t.Parallel()
	db := livebackend.MongoDatabase(t, "mg")
	var entered, concurrent, maxConcurrent atomic.Int32
	enteredCh := make(chan struct{}, 1)
	var reg migrations.Registry
	require.NoError(t, reg.Register(1, "slow", func(ctx context.Context, _ *mongo.Database) error {
		entered.Add(1)
		n := concurrent.Add(1)
		maxConcurrent.Store(max(maxConcurrent.Load(), n))
		enteredCh <- struct{}{}
		<-ctx.Done() // The budget ends; the step carries on regardless of it.
		concurrent.Add(-1)
		return nil
	}, nil))

	aDone := make(chan error, 1)
	go func() { aDone <- migrations.Run(t.Context(), db, migrations.WithRegistry(&reg)) }()
	<-enteredCh
	bErr := migrations.Run(t.Context(), db, migrations.WithRegistry(&reg))

	require.NoError(t, <-aDone, "the over-budget step is recorded")
	require.Equal(t, uint64(1), ledgerVersion(t, db))
	if bErr == nil { // B won the lease after A released it and found the version recorded.
		require.Equal(t, int32(1), entered.Load())
	} else {
		require.True(t, errors.Is(bErr, context.DeadlineExceeded) || errors.Is(bErr, context.Canceled), "unexpected: %v", bErr)
	}
	require.Equal(t, int32(1), entered.Load(), "the step is entered once")
	require.Equal(t, int32(1), maxConcurrent.Load(), "no overlap")
	require.NoError(t, migrations.Run(t.Context(), db, migrations.WithRegistry(&reg)))
	require.Equal(t, int32(1), entered.Load(), "a later run is a no-op")
}

// TestLeaseRenewedSeveralTimes: a step that runs through several renewals within its budget completes and is recorded.
func TestLeaseRenewedSeveralTimes(t *testing.T) {
	t.Parallel()
	db := livebackend.MongoDatabase(t, "mg")
	var reg migrations.Registry
	require.NoError(t, reg.Register(1, "renewed", func(ctx context.Context, d *mongo.Database) error {
		seen := map[time.Time]struct{}{}
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for len(seen) < 3 { // The initial confirmation and two heartbeats move the expiry.
			select {
			case <-ctx.Done():
				return fmt.Errorf("lease was not renewed: %w", ctx.Err())
			case <-ticker.C:
			}
			exp, err := leaseExpiry(ctx, d)
			if err != nil {
				return err
			}
			seen[exp] = struct{}{}
		}
		return nil
	}, nil))
	require.NoError(t, migrations.Run(t.Context(), db, migrations.WithRegistry(&reg)))
	require.Equal(t, uint64(1), ledgerVersion(t, db))
}
