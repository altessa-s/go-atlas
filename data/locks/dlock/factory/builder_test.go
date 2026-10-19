// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/locks/dlock/factory"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	lockconfig "github.com/altessa-s/go-atlas/config/lock"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	locknats "github.com/altessa-s/go-atlas/data/locks/dlock/providers/nats"
	mongodrv "go.mongodb.org/mongo-driver/v2/mongo"
	mongoopts "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// TestBuild_NatsMigrateBucketTTL pins that nats.migrateBucketTTL reaches the
// NATS provider: without it a bucket with a different key TTL is rejected and
// left alone, with it the bucket is migrated to the provider's lock TTL.
func TestBuild_NatsMigrateBucketTTL(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	nc := testhelpers.ConnectNATS(t, ns)
	js, err := jetstream.New(nc)
	require.NoError(t, err)

	const bucket = "factory-dlock"
	first, err := locknats.New(t.Context(), nc, locknats.WithBucket(bucket), locknats.WithTTL(time.Minute))
	require.NoError(t, err)
	t.Cleanup(func() { _ = first.Close(context.Background()) })

	cfg := func(migrate bool) *lockconfig.DistributionLock {
		return &lockconfig.DistributionLock{
			Provider: lockconfig.DistributionLockProviderNATS,
			NATS:     &lockconfig.DistributionLockNATS{Bucket: bucket, MigrateBucketTTL: migrate},
		}
	}

	_, err = factory.New(cfg(false)).UseNATSConn(nc).Build(t.Context())
	require.ErrorIs(t, err, locknats.ErrBucketTTLMismatch)
	require.Equal(t, time.Minute, testhelpers.KVBucketTTL(t, js, bucket), "a rejected build changed the bucket's TTL")

	dl, err := factory.New(cfg(true)).UseNATSConn(nc).Build(t.Context())
	require.NoError(t, err)
	require.NotNil(t, dl)
	require.Equal(t, locknats.DefaultBucketKeysTTL, testhelpers.KVBucketTTL(t, js, bucket), "migrateBucketTTL must update the bucket")
}

func TestBuild_NilNatsSection(t *testing.T) {
	t.Parallel()

	_, err := factory.New(&lockconfig.DistributionLock{Provider: lockconfig.DistributionLockProviderNATS}).Build(t.Context())
	require.Error(t, err)
}

// TestBuild_NatsStrictBucketStorage pins that nats.strictBucketStorage reaches
// the NATS provider: a bucket with another storage type is rejected instead of
// being adopted.
func TestBuild_NatsStrictBucketStorage(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	nc, js := testhelpers.ConnectJetStream(t, ns)

	const bucket = "factory-dlock-strict"
	_, err := js.CreateKeyValue(t.Context(), jetstream.KeyValueConfig{
		Bucket:  bucket,
		TTL:     locknats.DefaultBucketKeysTTL,
		Storage: jetstream.FileStorage,
	})
	require.NoError(t, err)

	cfg := func(strict bool) *lockconfig.DistributionLock {
		return &lockconfig.DistributionLock{
			Provider: lockconfig.DistributionLockProviderNATS,
			NATS:     &lockconfig.DistributionLockNATS{Bucket: bucket, StrictBucketStorage: strict},
		}
	}

	_, err = factory.New(cfg(true)).UseNATSConn(nc).Build(t.Context())
	require.ErrorIs(t, err, locknats.ErrBucketStorageMismatch)

	dl, err := factory.New(cfg(false)).UseNATSConn(nc).Build(t.Context())
	require.NoError(t, err, "without the flag the bucket is adopted")
	require.NotNil(t, dl)
}

// TestBuild_NatsStorage pins that nats.storage selects the storage type of the
// bucket the provider creates, memory by default.
func TestBuild_NatsStorage(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	nc, js := testhelpers.ConnectJetStream(t, ns)

	for bucket, storage := range map[string]storageconfig.KVStorageType{"locks-default": "", "locks-memory": storageconfig.KVStorageMemory, "locks-file": storageconfig.KVStorageFile} {
		dl, err := factory.New(&lockconfig.DistributionLock{
			Provider: lockconfig.DistributionLockProviderNATS,
			NATS:     &lockconfig.DistributionLockNATS{Bucket: bucket, Storage: storage},
		}).UseNATSConn(nc).Build(t.Context())
		require.NoError(t, err)
		require.NotNil(t, dl)

		want := jetstream.MemoryStorage
		if storage == storageconfig.KVStorageFile {
			want = jetstream.FileStorage
		}
		require.Equal(t, want, testhelpers.KVBucketStorage(t, js, bucket), bucket)
	}
}

func TestBuild_Mongodb(t *testing.T) {
	t.Parallel()
	client, err := mongodrv.Connect(mongoopts.Client().ApplyURI("mongodb://127.0.0.1:1"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	mongoCfg := lockconfig.DefaultDistributionLockMongo()

	t.Run("requires_database", func(t *testing.T) {
		t.Parallel()
		_, err := factory.New(&lockconfig.DistributionLock{Provider: lockconfig.DistributionLockProviderMongo, Mongo: &mongoCfg}).Build(t.Context())
		require.ErrorContains(t, err, "mongo database")
	})
	t.Run("requires_section", func(t *testing.T) {
		t.Parallel()
		_, err := factory.New(&lockconfig.DistributionLock{Provider: lockconfig.DistributionLockProviderMongo}).
			UseMongoDB(client.Database("x")).Build(t.Context())
		require.Error(t, err)
	})
	t.Run("builds_without_io", func(t *testing.T) {
		t.Parallel()
		dl, err := factory.New(&lockconfig.DistributionLock{Provider: lockconfig.DistributionLockProviderMongo, Mongo: &mongoCfg}).
			UseMongoDB(client.Database("x")).Build(t.Context())
		require.NoError(t, err)
		require.NotNil(t, dl)
	})
}

// TestBuild_MongoOptionsFromConfig checks that the provider settings reach
// the provider (an invalid one fails Build) and that EnsureIndexes is the only
// setting that makes Build talk to MongoDB.
func TestBuild_MongoOptionsFromConfig(t *testing.T) {
	t.Parallel()
	client, err := mongodrv.Connect(mongoopts.Client().ApplyURI("mongodb://127.0.0.1:1").
		SetServerSelectionTimeout(100 * time.Millisecond))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	build := func(cfg lockconfig.DistributionLockMongo) error {
		_, err := factory.New(&lockconfig.DistributionLock{Provider: lockconfig.DistributionLockProviderMongo, Mongo: &cfg}).
			UseMongoDB(client.Database("x")).Build(t.Context())
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

func TestBuild_SQL(t *testing.T) {
	t.Parallel()
	sqlCfg := func(ensure bool) *lockconfig.DistributionLock {
		return &lockconfig.DistributionLock{Provider: lockconfig.DistributionLockProviderSQL,
			SQL: &lockconfig.DistributionLockSQL{Dialect: lockconfig.SQLDialectPostgres, Table: "app.locks", EnsureSchema: ensure}}
	}

	t.Run("requires_database", func(t *testing.T) {
		t.Parallel()
		_, err := factory.New(sqlCfg(false)).Build(t.Context())
		require.ErrorContains(t, err, "sql database")
	})
	t.Run("requires_section", func(t *testing.T) {
		t.Parallel()
		db, _ := testhelpers.NewFakeSQL(t, nil)
		_, err := factory.New(&lockconfig.DistributionLock{Provider: lockconfig.DistributionLockProviderSQL}).
			UseSQLDB(db).Build(t.Context())
		require.Error(t, err)
	})
	for _, ensure := range []bool{false, true} {
		t.Run("ensure_schema_"+strconv.FormatBool(ensure), func(t *testing.T) {
			t.Parallel()
			db, fake := testhelpers.NewFakeSQL(t, nil)
			dl, err := factory.New(sqlCfg(ensure)).UseSQLDB(db).Build(t.Context())
			require.NoError(t, err)
			require.NotNil(t, dl)
			var ddl int
			for _, c := range fake.Calls() {
				if strings.Contains(c.Query, `CREATE TABLE IF NOT EXISTS "app"."locks"`) {
					ddl++
				}
			}
			require.Equal(t, map[bool]int{false: 0, true: 1}[ensure], ddl)
		})
	}
}
