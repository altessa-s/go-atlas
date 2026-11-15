// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/ext/horai/clickhouse"

	schedulerconfig "github.com/altessa-s/go-atlas/config/scheduler"
	schedulerfactory "github.com/altessa-s/go-atlas/factory/ext/horai"
	chstorage "github.com/altessa-s/go-horai/storage/clickhouse"
)

// chConn records the statements the history storage executes, failing each
// with err.
type chConn struct {
	err   error
	execs []string
}

func (c *chConn) Query(context.Context, string, ...any) (driver.Rows, error) {
	return nil, errors.New("not served")
}

func (c *chConn) Exec(_ context.Context, query string, _ ...any) error {
	c.execs = append(c.execs, query)
	return c.err
}

func clickHouseHistoryConfig(ch *schedulerconfig.HistoryClickHouseConfig) *schedulerconfig.Config {
	cfg := schedulerconfig.Default()
	cfg.HistoryStorage = &schedulerconfig.HistoryStorageConfig{Type: schedulerconfig.HistoryStorageTypeClickHouse, ClickHouse: ch}
	return &cfg
}

// build builds a scheduler with the ClickHouse history storage registered
// over conn.
func build(t *testing.T, cfg *schedulerconfig.Config, conn chstorage.Conn) error {
	t.Helper()
	s, err := schedulerfactory.New(cfg).
		UseHistoryStorage(schedulerconfig.HistoryStorageTypeClickHouse, factory.Builder(conn)).
		Build(t.Context())
	if err == nil {
		require.NotNil(t, s)
	}
	return err
}

func TestBuilder(t *testing.T) {
	t.Parallel()

	t.Run("requires_conn", func(t *testing.T) {
		t.Parallel()
		err := build(t, clickHouseHistoryConfig(&schedulerconfig.HistoryClickHouseConfig{TableName: "h", Engine: "MergeTree"}), nil)
		require.ErrorIs(t, err, factory.ErrConnRequired)
	})

	t.Run("requires_section", func(t *testing.T) {
		t.Parallel()
		err := build(t, clickHouseHistoryConfig(nil), &chConn{})
		require.Error(t, err)
	})

	t.Run("rejects_invalid_table", func(t *testing.T) {
		t.Parallel()
		err := build(t, clickHouseHistoryConfig(&schedulerconfig.HistoryClickHouseConfig{TableName: "db.h", Engine: "MergeTree"}), &chConn{})
		require.ErrorIs(t, err, chstorage.ErrInvalidIdentifier)
	})

	t.Run("builds_without_io", func(t *testing.T) {
		t.Parallel()
		conn := &chConn{}
		err := build(t, clickHouseHistoryConfig(&schedulerconfig.HistoryClickHouseConfig{TableName: "h", Engine: "MergeTree"}), conn)
		require.NoError(t, err)
		require.Empty(t, conn.execs)
	})

	t.Run("ensure_schema_defaults_ttl_to_retention", func(t *testing.T) {
		t.Parallel()
		conn := &chConn{}
		cfg := clickHouseHistoryConfig(&schedulerconfig.HistoryClickHouseConfig{TableName: "h", Engine: "MergeTree", EnsureSchema: true})
		cfg.HistoryRetention = 48 * time.Hour
		require.NoError(t, build(t, cfg, conn))
		want, err := chstorage.SchemaDDL("h", "MergeTree", "", 48*time.Hour)
		require.NoError(t, err)
		require.Equal(t, []string{want}, conn.execs)
	})

	t.Run("ensure_schema_explicit_ttl", func(t *testing.T) {
		t.Parallel()
		conn := &chConn{}
		require.NoError(t, build(t, clickHouseHistoryConfig(&schedulerconfig.HistoryClickHouseConfig{
			TableName: "h", Engine: "MergeTree", TTL: time.Hour, EnsureSchema: true,
		}), conn))
		want, err := chstorage.SchemaDDL("h", "MergeTree", "", time.Hour)
		require.NoError(t, err)
		require.Equal(t, []string{want}, conn.execs)
	})

	t.Run("ensure_schema_failure", func(t *testing.T) {
		t.Parallel()
		errDDL := errors.New("ddl failed")
		err := build(t, clickHouseHistoryConfig(&schedulerconfig.HistoryClickHouseConfig{
			TableName: "h", Engine: "MergeTree", EnsureSchema: true,
		}), &chConn{err: errDDL})
		require.ErrorIs(t, err, errDDL)
	})
}
