// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/coord/dlock/providers/mongo"

	lockconfig "github.com/altessa-s/go-atlas/config/lock"
	dlockfactory "github.com/altessa-s/go-atlas/factory/coord/dlock"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	mongoopts "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// newBuilder returns a dlock builder with the mongo provider registered on db.
func newBuilder(cfg *lockconfig.DistributionLock, db *mongodrv.Database) *dlockfactory.DLockBuilder {
	return dlockfactory.New(cfg).UseProvider(lockconfig.DistributionLockProviderMongo, factory.Builder(db))
}

func TestBuilder_Mongodb(t *testing.T) {
	t.Parallel()
	client, err := mongodrv.Connect(mongoopts.Client().ApplyURI("mongodb://127.0.0.1:1"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Disconnect(context.WithoutCancel(t.Context())) })
	mongoCfg := lockconfig.DefaultDistributionLockMongo()

	t.Run("requires_database", func(t *testing.T) {
		t.Parallel()
		_, err := newBuilder(&lockconfig.DistributionLock{Provider: lockconfig.DistributionLockProviderMongo, Mongo: &mongoCfg}, nil).Build(t.Context())
		require.ErrorIs(t, err, factory.ErrClientRequired)
	})
	t.Run("requires_section", func(t *testing.T) {
		t.Parallel()
		_, err := newBuilder(&lockconfig.DistributionLock{Provider: lockconfig.DistributionLockProviderMongo}, client.Database("x")).
			Build(t.Context())
		require.ErrorIs(t, err, factory.ErrConfigRequired)
		require.ErrorIs(t, err, corefactory.ErrConfigRequired)
	})
	t.Run("builds_without_io", func(t *testing.T) {
		t.Parallel()
		dl, err := newBuilder(&lockconfig.DistributionLock{Provider: lockconfig.DistributionLockProviderMongo, Mongo: &mongoCfg},
			client.Database("x")).Build(t.Context())
		require.NoError(t, err)
		require.NotNil(t, dl)
	})
}

// TestBuilder_OptionsFromConfig checks that the provider settings reach
// the provider (an invalid one fails Build) and that EnsureIndexes is the only
// setting that makes Build talk to MongoDB.
func TestBuilder_OptionsFromConfig(t *testing.T) {
	t.Parallel()
	client, err := mongodrv.Connect(mongoopts.Client().ApplyURI("mongodb://127.0.0.1:1").
		SetServerSelectionTimeout(100 * time.Millisecond))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Disconnect(context.WithoutCancel(t.Context())) })
	build := func(cfg lockconfig.DistributionLockMongo) error {
		_, err := newBuilder(&lockconfig.DistributionLock{Provider: lockconfig.DistributionLockProviderMongo, Mongo: &cfg},
			client.Database("x")).Build(t.Context())
		return err
	}

	hard := lockconfig.DefaultDistributionLockMongo()
	hard.WithoutRenewal, hard.ExpiredCleanup, hard.ExpiredCleanupGrace = true, true, time.Minute
	require.NoError(t, build(hard), "a hard lease with cleanup builds without I/O")

	tooShort := lockconfig.DefaultDistributionLockMongo()
	tooShort.TTL = 500 * time.Microsecond
	require.Error(t, build(tooShort), "the TTL reaches the provider")

	badRatio := lockconfig.DefaultDistributionLockMongo()
	badRatio.TTL, badRatio.RenewRatio = 2*time.Millisecond, 0.1
	require.Error(t, build(badRatio), "the renew ratio reaches the provider")
	badRatio.WithoutRenewal = true
	require.NoError(t, build(badRatio), "a hard lease ignores the renew ratio")

	negative := lockconfig.DefaultDistributionLockMongo()
	negative.ExpiredCleanup, negative.ExpiredCleanupGrace = true, -time.Second
	require.Error(t, build(negative), "the cleanup grace reaches the provider")

	ensure := lockconfig.DefaultDistributionLockMongo()
	ensure.EnsureIndexes = true
	require.Error(t, build(ensure), "EnsureIndexes reconciles the index during Build")
}
