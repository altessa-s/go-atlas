// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	coreruntime "github.com/altessa-s/go-atlas-core/runtime"
	idempotencyconfig "github.com/altessa-s/go-atlas/config/idempotency"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	factory "github.com/altessa-s/go-atlas/factory/reliability/idempotency"
	sqldb "github.com/altessa-s/go-atlas/reliability/idempotency/storages/sqldb"
)

func sqlConfig() *idempotencyconfig.Config {
	return storageCfg(&idempotencyconfig.StorageConfig{Type: idempotencyconfig.StorageTypeSQL,
		SQL: &idempotencyconfig.SQLStorageConfig{Dialect: "postgres", Table: "custom_keys"}})
}

func TestSQLCleanupOwnership(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, func(q string, _ []any) testhelpers.FakeSQLReply {
		if strings.HasPrefix(q, "DELETE") {
			return testhelpers.FakeSQLReply{}
		}
		return testhelpers.FakeSQLReply{Affected: 1}
	})
	hooks := &coreruntime.HookGroup{}
	t.Cleanup(func() { require.NoError(t, hooks.Shutdown(context.WithoutCancel(t.Context()))) })
	cfg := sqlConfig()
	cfg.Storage.SQL.PurgeInterval = time.Millisecond
	cfg.Storage.SQL.PurgeBatchSize = 7
	k, err := factory.New(cfg).UseSQLDB(db).UseShutdownHooks(hooks).Build(t.Context())
	require.NoError(t, err)
	require.Empty(t, fake.Calls(), "Build must not start SQL cleanup")
	_, _, err = k.AttemptLock(t.Context(), "key")
	require.NoError(t, err)
	testhelpers.WaitFor(t, time.Second, func() bool {
		for _, call := range fake.Calls() {
			if strings.HasPrefix(call.Query, "DELETE") {
				require.Equal(t, []any{int64(7)}, call.Args)
				return true
			}
		}
		return false
	}, "factory must pass the cleanup interval")
	require.NoError(t, hooks.Shutdown(t.Context()))
	calls := len(fake.Calls())
	_, _, err = k.AttemptLock(t.Context(), "after-shutdown")
	require.NoError(t, err, "shutdown must leave the injected database open")
	require.Len(t, fake.Calls(), calls+1)
}

func TestSQLDependencies(t *testing.T) {
	t.Parallel()
	_, err := factory.New(sqlConfig()).Build(t.Context())
	require.ErrorIs(t, err, corefactory.ErrDependencyRequired)
	_, err = factory.New(sqlConfig()).UseSQLDB(nil).Build(t.Context())
	require.ErrorIs(t, err, corefactory.ErrDependencyRequired)
	db, _ := testhelpers.NewFakeSQL(t, nil)
	cfg := sqlConfig()
	cfg.Storage.SQL = nil
	_, err = factory.New(cfg).UseSQLDB(db).Build(t.Context())
	require.ErrorIs(t, err, corefactory.ErrConfigRequired)
	require.Contains(t, err.Error(), "idempotency storage.sql")
}

func TestSQLBuild(t *testing.T) {
	t.Parallel()
	for _, dialect := range []string{"postgres", "mysql"} {
		for _, ensure := range []bool{false, true} {
			name := dialect + "/migrations"
			if ensure {
				name = dialect + "/ensure"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				db, fake := testhelpers.NewFakeSQL(t, nil)
				cfg := sqlConfig()
				cfg.Storage.SQL.Dialect = dialect
				cfg.Storage.SQL.EnsureSchema = ensure
				cfg.TTL = 2 * time.Hour
				k, err := factory.New(cfg).UseSQLDB(db).Build(t.Context())
				require.NoError(t, err)
				require.NotNil(t, k)
				if ensure {
					require.NotEmpty(t, fake.Calls())
				} else {
					require.Empty(t, fake.Calls(), "migrations mode must perform no startup I/O")
				}
				ok, _, err := k.AttemptLock(t.Context(), "key")
				require.NoError(t, err)
				require.True(t, ok)
				calls := fake.Calls()
				insert := calls[len(calls)-1]
				require.Contains(t, insert.Query, "custom_keys")
				if dialect == "postgres" {
					require.Equal(t, float64(7200), insert.Args[2])
				} else {
					require.Equal(t, int64((2 * time.Hour).Microseconds()), insert.Args[2])
				}
			})
		}
	}
}

func TestSQLBuildFailures(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, func(string, []any) testhelpers.FakeSQLReply {
		return testhelpers.FakeSQLReply{Err: errBuild}
	})
	cfg := sqlConfig()
	cfg.Storage.SQL.EnsureSchema = true
	_, err := factory.New(cfg).UseSQLDB(db).Build(t.Context())
	require.ErrorIs(t, err, errBuild)
	require.NotEmpty(t, fake.Calls())
	cfg = sqlConfig()
	cfg.Storage.SQL.Dialect = "oracle"
	_, err = factory.New(cfg).UseSQLDB(db).Build(t.Context())
	require.ErrorIs(t, err, sqldb.ErrUnsupportedDialect)
	require.Contains(t, err.Error(), "create SQL idempotency storage")
	cfg = sqlConfig()
	cfg.Storage.SQL.Table = "bad;table"
	_, err = factory.New(cfg).UseSQLDB(db).Build(t.Context())
	require.ErrorIs(t, err, sqldb.ErrInvalidTableName)
	require.Contains(t, err.Error(), "create SQL idempotency storage")
}

func TestBuildSQLHonorsStartupCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	db, fake := testhelpers.NewFakeSQL(t, nil)
	cfg := sqlConfig()
	cfg.Storage.SQL.EnsureSchema = true
	_, err := factory.New(cfg).UseSQLDB(db).Build(ctx)
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, fake.Calls())
}
