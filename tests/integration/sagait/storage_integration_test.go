// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sagait_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/data/saga"
	"github.com/altessa-s/go-atlas/data/saga/storages/sqldb"
	"github.com/altessa-s/go-atlas/data/saga/storages/storagetest"

	sagaconfig "github.com/altessa-s/go-atlas/config/saga"
	sagafactory "github.com/altessa-s/go-atlas/data/saga/factory"
	mongostore "github.com/altessa-s/go-atlas/data/saga/storages/mongo"
	redisstore "github.com/altessa-s/go-atlas/data/saga/storages/redis"
	goredis "github.com/redis/go-redis/v9"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// envOr returns the environment override for a connection setting, or the
// default that matches tests/integration/docker-compose.yml.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// sqlTarget is one database/sql server of the docker stack.
type sqlTarget struct {
	name, driver, dsn string
	dialect           sqldb.Dialect
}

func sqlTargets() []sqlTarget {
	return []sqlTarget{
		{"postgres", "pgx", envOr("POSTGRES_DSN", "postgres://atlas:atlas@127.0.0.1:15432/atlas?sslmode=disable"), sqldb.DialectPostgres},
		{"mariadb", "mysql", envOr("MARIADB_DSN", "atlas:atlas@tcp(127.0.0.1:13306)/atlas"), sqldb.DialectMySQL},
		{"mysql", "mysql", envOr("MYSQL_DSN", "atlas:atlas@tcp(127.0.0.1:13307)/atlas"), sqldb.DialectMySQL},
	}
}

// open connects to the server and returns a throwaway table name, dropped on
// cleanup. It skips when the server is unreachable.
func (s sqlTarget) open(tb testing.TB) (*sql.DB, string) {
	tb.Helper()
	db, err := sql.Open(s.driver, s.dsn)
	require.NoError(tb, err)
	ctx, cancel := context.WithTimeout(tb.Context(), 3*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		// The DSN is not printed: an override may carry a password.
		tb.Skipf("%s unreachable (%v) — start it with: make integration-up", s.name, err)
	}
	table := tableName(tb)
	tb.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DROP TABLE IF EXISTS "+table)
		_ = db.Close()
	})
	return db, table
}

// store builds a store over a throwaway table, created with EnsureSchema
// (twice, to prove idempotence).
func (s sqlTarget) store(tb testing.TB) saga.Storage {
	tb.Helper()
	db, table := s.open(tb)
	store, err := sqldb.New(db, s.dialect, sqldb.WithTableName(table))
	require.NoError(tb, err)
	require.NoError(tb, store.EnsureSchema(tb.Context()))
	require.NoError(tb, store.EnsureSchema(tb.Context()))
	return store
}

// tableName returns a table name unique across tests and test processes. It is
// lower case because the cleanup DROP is unquoted and PostgreSQL folds an
// unquoted name to lower case.
func tableName(tb testing.TB) string {
	tb.Helper()
	safe := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return '_'
	}, strings.ToLower(tb.Name()))
	const maxPrefix = 40
	return "saga_" + safe[:min(len(safe), maxPrefix)] + "_" + strings.ToLower(rand.Text()[:8])
}

// mongoURI matches tests/integration/docker-compose.yml; directConnection is
// required because the single-node replica set advertises its in-container
// address.
func mongoURI() string {
	return envOr("MONGO_URI", "mongodb://127.0.0.1:27019/?directConnection=true")
}

// newMongoStore gives the test a throwaway database, dropped on cleanup.
func newMongoStore(tb testing.TB) saga.Storage {
	tb.Helper()
	client, err := mongo.Connect(mongoOptions.Client().ApplyURI(mongoURI()).SetServerSelectionTimeout(3 * time.Second))
	if err != nil {
		tb.Skipf("MongoDB unreachable (%v) — start it with: make integration-up", err)
	}
	if err := client.Ping(tb.Context(), nil); err != nil {
		_ = client.Disconnect(context.Background())
		tb.Skipf("MongoDB unreachable (%v) — start it with: make integration-up", err)
	}
	db := client.Database(tableName(tb))
	tb.Cleanup(func() {
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})
	store, err := mongostore.New(db)
	require.NoError(tb, err)
	return store
}

// newRedisStore gives the test a key prefix of its own on the shared server.
func newRedisStore(tb testing.TB) saga.Storage {
	tb.Helper()
	client := goredis.NewClient(&goredis.Options{Addr: envOr("REDIS_ADDR", "127.0.0.1:16379")})
	ctx, cancel := context.WithTimeout(tb.Context(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		tb.Skipf("Redis unreachable (%v) — start it with: make integration-up", err)
	}
	prefix := tableName(tb) + ":"
	tb.Cleanup(func() {
		keys, _ := client.Keys(context.Background(), prefix+"*").Result()
		if len(keys) > 0 {
			_ = client.Del(context.Background(), keys...).Err()
		}
		_ = client.Close()
	})
	return redisstore.New(client, redisstore.WithKeyPrefix(prefix))
}

// TestStorageContract runs the saga storage contract suite on every live
// backend, each contract over its own isolated store.
func TestStorageContract(t *testing.T) {
	t.Parallel()
	backends := map[string]func(testing.TB) saga.Storage{"mongodb": newMongoStore, "redis": newRedisStore}
	for _, target := range sqlTargets() {
		backends[target.name] = target.store
	}
	for name, newStore := range backends {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			storagetest.Run(t, newStore)
		})
	}
}

type order struct {
	Steps []string
}

// TestOrchestratorOverSQL builds an orchestrator through the factory on each
// SQL server, with the factory creating the schema, and runs a saga whose
// second step fails, so it commits, compensates and persists every transition.
func TestOrchestratorOverSQL(t *testing.T) {
	t.Parallel()
	for _, target := range sqlTargets() {
		t.Run(target.name, func(t *testing.T) {
			t.Parallel()
			db, table := target.open(t)
			cfg := sagaconfig.Default()
			cfg.MaxStepAttempts = 1
			cfg.Storage = &sagaconfig.StorageConfig{
				Type: sagaconfig.StorageTypeSQL,
				SQL:  &sagaconfig.SQLStorageConfig{Dialect: string(target.dialect), Table: table, EnsureSchema: true},
			}
			def := saga.NewDefinition[order]("place-order").
				Step("reserve", func(_ context.Context, o *order) error {
					o.Steps = append(o.Steps, "reserve")
					return nil
				}).Compensate(func(_ context.Context, o *order) error {
				o.Steps = append(o.Steps, "release")
				return nil
			}).
				Step("charge", func(context.Context, *order) error { return context.DeadlineExceeded }).
				MustBuild()

			orch, err := sagafactory.New(&cfg, def).UseSQLDB(db).Build()
			require.NoError(t, err)

			inst, err := orch.Start(t.Context(), "order-ü-1", order{})
			require.Error(t, err)
			require.NotNil(t, inst)
			require.Equal(t, saga.StatusCompensated, inst.Status)

			store, err := sqldb.New(db, target.dialect, sqldb.WithTableName(table))
			require.NoError(t, err)
			stored, err := store.Get(t.Context(), "order-ü-1")
			require.NoError(t, err)
			require.Equal(t, saga.StatusCompensated, stored.Status)
			require.Equal(t, inst.Version, stored.Version)
			require.NotEmpty(t, stored.Steps)
		})
	}
}
