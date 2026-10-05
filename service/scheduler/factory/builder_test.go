// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/service/scheduler/factory"
	"github.com/altessa-s/go-atlas/service/scheduler/storages/sqldb"
)

// noConn is a connector that never connects: building the SQL storage
// performs no I/O, so a *sql.DB that cannot reach a server is enough.
type noConn struct{}

func (noConn) Connect(context.Context) (driver.Conn, error) { return nil, errors.New("not connected") }
func (noConn) Driver() driver.Driver                        { return nil }

func sqlConfig(dialect string) *config.Scheduler {
	cfg := config.DefaultScheduler()
	storageCfg := config.DefaultSchedulerStorageSQLConfig()
	storageCfg.Dialect = dialect
	cfg.Storage = &config.SchedulerStorageConfig{Type: config.SchedulerStorageTypeSQL, SQL: &storageCfg}
	return &cfg
}

func TestBuildSQLStorage(t *testing.T) {
	t.Parallel()
	db := sql.OpenDB(noConn{})
	t.Cleanup(func() { _ = db.Close() })

	t.Run("requires_db", func(t *testing.T) {
		t.Parallel()
		_, err := factory.New(sqlConfig(config.SchedulerSQLDialectPostgres)).Build()
		require.ErrorContains(t, err, "sql database")
	})

	t.Run("builds", func(t *testing.T) {
		t.Parallel()
		s, err := factory.New(sqlConfig(config.SchedulerSQLDialectMySQL)).UseSQLDB(db).Build()
		require.NoError(t, err)
		require.NotNil(t, s)
	})

	t.Run("rejects_unknown_dialect", func(t *testing.T) {
		t.Parallel()
		_, err := factory.New(sqlConfig("oracle")).UseSQLDB(db).Build()
		require.ErrorIs(t, err, sqldb.ErrUnsupportedDialect)
	})

	t.Run("requires_section", func(t *testing.T) {
		t.Parallel()
		cfg := sqlConfig(config.SchedulerSQLDialectPostgres)
		cfg.Storage.SQL = nil
		_, err := factory.New(cfg).UseSQLDB(db).Build()
		require.Error(t, err)
	})
}
