// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-horai/storage/memory"
	"github.com/altessa-s/go-horai/storage/sqldb"

	"github.com/altessa-s/go-atlas/factory/ext/horai"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	schedulerconfig "github.com/altessa-s/go-atlas/config/scheduler"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	horai "github.com/altessa-s/go-horai"
)

// noConn is a connector that never connects: building the SQL storage
// performs no I/O, so a *sql.DB that cannot reach a server is enough.
type noConn struct{}

var errNotConnected = errors.New("not connected")

func (noConn) Connect(context.Context) (driver.Conn, error) { return nil, errNotConnected }
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
		_, err := factory.New(sqlConfig(schedulerconfig.SQLDialectPostgres)).Build(t.Context())
		require.ErrorIs(t, err, corefactory.ErrDependencyRequired)
	})

	t.Run("builds", func(t *testing.T) {
		t.Parallel()
		s, err := factory.New(sqlConfig(schedulerconfig.SQLDialectMySQL)).UseSQLDB(db).Build(t.Context())
		require.NoError(t, err)
		require.NotNil(t, s)
	})

	t.Run("rejects_unknown_dialect", func(t *testing.T) {
		t.Parallel()
		_, err := factory.New(sqlConfig("oracle")).UseSQLDB(db).Build(t.Context())
		require.ErrorIs(t, err, sqldb.ErrUnsupportedDialect)
	})

	t.Run("ensure_schema", func(t *testing.T) {
		t.Parallel()
		cfg := sqlConfig(schedulerconfig.SQLDialectPostgres)
		cfg.Storage.SQL.EnsureSchema = true
		fakeDB, fake := testhelpers.NewFakeSQL(t, nil)
		s, err := factory.New(cfg).UseSQLDB(fakeDB).Build(t.Context())
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
		_, err := factory.New(cfg).UseSQLDB(db).Build(t.Context())
		require.ErrorIs(t, err, errNotConnected)
	})

	// The caller's context bounds the provisioning: a canceled startup must
	// not run DDL on a detached context.
	t.Run("ensure_schema_honors_context", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		cfg := sqlConfig(schedulerconfig.SQLDialectPostgres)
		cfg.Storage.SQL.EnsureSchema = true
		fakeDB, fake := testhelpers.NewFakeSQL(t, nil)
		_, err := factory.New(cfg).UseSQLDB(fakeDB).Build(ctx)
		require.ErrorIs(t, err, context.Canceled)
		require.Empty(t, fake.Calls())
	})

	t.Run("requires_section", func(t *testing.T) {
		t.Parallel()
		cfg := sqlConfig(schedulerconfig.SQLDialectPostgres)
		cfg.Storage.SQL = nil
		_, err := factory.New(cfg).UseSQLDB(db).Build(t.Context())
		require.Error(t, err)
	})
}

var errStorage = errors.New("storage failed")

func TestUseStorage(t *testing.T) {
	t.Parallel()

	for _, typ := range []schedulerconfig.StorageType{schedulerconfig.StorageTypeMongo, schedulerconfig.StorageTypeRedis} {
		t.Run(string(typ), func(t *testing.T) {
			t.Parallel()
			cfg := schedulerconfig.Default()
			cfg.Storage = &schedulerconfig.StorageConfig{Type: typ}

			t.Run("unregistered_fails", func(t *testing.T) {
				t.Parallel()
				_, err := factory.New(&cfg).Build(t.Context())
				require.Error(t, err)
			})

			t.Run("nil_function_fails", func(t *testing.T) {
				t.Parallel()
				_, err := factory.New(&cfg).UseStorage(typ, nil).Build(t.Context())
				require.Error(t, err)
			})

			t.Run("function_error_propagates", func(t *testing.T) {
				t.Parallel()
				failing := func(context.Context, *schedulerconfig.Config) (horai.Store, error) {
					return nil, errStorage
				}
				_, err := factory.New(&cfg).UseStorage(typ, failing).Build(t.Context())
				require.ErrorIs(t, err, errStorage)
			})

			t.Run("registered_function_used", func(t *testing.T) {
				t.Parallel()
				failing := func(context.Context, *schedulerconfig.Config) (horai.Store, error) {
					return nil, errStorage
				}
				called := false
				registered := func(_ context.Context, got *schedulerconfig.Config) (horai.Store, error) {
					require.Same(t, &cfg, got)
					called = true
					return memory.New(memory.WithMaxHistoryPerTask(1))
				}
				s, err := factory.New(&cfg).UseStorage(typ, failing).UseStorage(typ, registered).Build(t.Context())
				require.NoError(t, err, "a later registration replaces the earlier one")
				require.NotNil(t, s)
				require.True(t, called)
			})
		})
	}
}
