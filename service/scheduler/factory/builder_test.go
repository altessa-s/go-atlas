// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/service/scheduler/factory"
	"github.com/altessa-s/go-atlas/service/scheduler/storages/sqldb"

	schedulerconfig "github.com/altessa-s/go-atlas/config/scheduler"
	mongooptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// noConn is a connector that never connects: building the SQL storage
// performs no I/O, so a *sql.DB that cannot reach a server is enough.
type noConn struct{}

func (noConn) Connect(context.Context) (driver.Conn, error) { return nil, errors.New("not connected") }
func (noConn) Driver() driver.Driver                        { return nil }

func sqlConfig(dialect string) *schedulerconfig.Config {
	cfg := schedulerconfig.Default()
	storageCfg := schedulerconfig.DefaultStorageSQLConfig()
	storageCfg.Dialect = dialect
	cfg.Storage = &schedulerconfig.StorageConfig{Type: schedulerconfig.StorageTypeSQL, SQL: &storageCfg}
	return &cfg
}

func TestBuildSQLStorage(t *testing.T) {
	t.Parallel()
	db := sql.OpenDB(noConn{})
	t.Cleanup(func() { _ = db.Close() })

	t.Run("requires_db", func(t *testing.T) {
		t.Parallel()
		_, err := factory.New(sqlConfig(schedulerconfig.SQLDialectPostgres)).Build()
		require.ErrorContains(t, err, "sql database")
	})

	t.Run("builds", func(t *testing.T) {
		t.Parallel()
		s, err := factory.New(sqlConfig(schedulerconfig.SQLDialectMySQL)).UseSQLDB(db).Build()
		require.NoError(t, err)
		require.NotNil(t, s)
	})

	t.Run("rejects_unknown_dialect", func(t *testing.T) {
		t.Parallel()
		_, err := factory.New(sqlConfig("oracle")).UseSQLDB(db).Build()
		require.ErrorIs(t, err, sqldb.ErrUnsupportedDialect)
	})

	t.Run("ensure_schema", func(t *testing.T) {
		t.Parallel()
		cfg := sqlConfig(schedulerconfig.SQLDialectPostgres)
		cfg.Storage.SQL.EnsureSchema = true
		fakeDB, fake := testhelpers.NewFakeSQL(t, nil)
		s, err := factory.New(cfg).UseSQLDB(fakeDB).Build()
		require.NoError(t, err)
		require.NotNil(t, s)
		var ddl int
		for _, c := range fake.Calls() {
			if strings.HasPrefix(c.Query, "CREATE TABLE IF NOT EXISTS") {
				ddl++
			}
		}
		require.Equal(t, 2, ddl, "both tables are created")
	})

	t.Run("ensure_schema_failure", func(t *testing.T) {
		t.Parallel()
		cfg := sqlConfig(schedulerconfig.SQLDialectMySQL)
		cfg.Storage.SQL.EnsureSchema = true
		_, err := factory.New(cfg).UseSQLDB(db).Build()
		require.ErrorContains(t, err, "not connected")
	})

	t.Run("requires_section", func(t *testing.T) {
		t.Parallel()
		cfg := sqlConfig(schedulerconfig.SQLDialectPostgres)
		cfg.Storage.SQL = nil
		_, err := factory.New(cfg).UseSQLDB(db).Build()
		require.Error(t, err)
	})
}

func TestBuildRedisStorage(t *testing.T) {
	t.Parallel()
	errDial := errors.New("not connected")
	client := redis.NewClient(&redis.Options{
		Dialer:     func(context.Context, string, string) (net.Conn, error) { return nil, errDial },
		MaxRetries: -1,
	})
	t.Cleanup(func() { _ = client.Close() })

	redisConfig := func(ensureIndexes bool) *schedulerconfig.Config {
		cfg := schedulerconfig.Default()
		storageCfg := schedulerconfig.DefaultStorageRedisConfig()
		storageCfg.EnsureIndexes = ensureIndexes
		cfg.Storage = &schedulerconfig.StorageConfig{Type: schedulerconfig.StorageTypeRedis, Redis: &storageCfg}
		return &cfg
	}

	t.Run("builds_without_io", func(t *testing.T) {
		t.Parallel()
		s, err := factory.New(redisConfig(false)).UseRedisClient(client).Build()
		require.NoError(t, err)
		require.NotNil(t, s)
	})

	t.Run("ensure_indexes_failure", func(t *testing.T) {
		t.Parallel()
		_, err := factory.New(redisConfig(true)).UseRedisClient(client).Build()
		require.ErrorIs(t, err, errDial)
	})
}

func TestBuildMongoStorage(t *testing.T) {
	t.Parallel()
	// Connect performs no I/O; server selection against the closed port fails
	// fast once an operation runs.
	client, err := mongo.Connect(mongooptions.Client().
		ApplyURI("mongodb://127.0.0.1:1/?directConnection=true").
		SetServerSelectionTimeout(100 * time.Millisecond))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	db := client.Database("scheduler_factory_test")

	mongoConfig := func(ensureIndexes bool) *schedulerconfig.Config {
		cfg := schedulerconfig.Default()
		storageCfg := schedulerconfig.DefaultStorageMongoConfig()
		storageCfg.EnsureIndexes = ensureIndexes
		cfg.Storage = &schedulerconfig.StorageConfig{Type: schedulerconfig.StorageTypeMongo, Mongodb: &storageCfg}
		return &cfg
	}

	t.Run("builds_without_io", func(t *testing.T) {
		t.Parallel()
		s, err := factory.New(mongoConfig(false)).UseMongoDb(db).Build()
		require.NoError(t, err)
		require.NotNil(t, s)
	})

	t.Run("ensure_indexes_failure", func(t *testing.T) {
		t.Parallel()
		_, err := factory.New(mongoConfig(true)).UseMongoDb(db).Build()
		require.True(t, mongo.IsTimeout(err), "want server selection timeout, got %v", err)
	})
}
