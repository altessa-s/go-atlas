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

	"github.com/altessa-s/go-atlas/service/scheduler/factory"

	schedulerconfig "github.com/altessa-s/go-atlas/config/scheduler"
	chstorage "github.com/altessa-s/go-atlas/service/scheduler/storages/clickhouse"
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

func TestBuildClickHouseHistoryStorage(t *testing.T) {
	t.Parallel()

	t.Run("requires_conn", func(t *testing.T) {
		t.Parallel()
		_, err := factory.New(clickHouseHistoryConfig(&schedulerconfig.HistoryClickHouseConfig{TableName: "h", Engine: "MergeTree"})).Build()
		require.Error(t, err)
	})

	t.Run("requires_section", func(t *testing.T) {
		t.Parallel()
		_, err := factory.New(clickHouseHistoryConfig(nil)).UseClickHouseConn(&chConn{}).Build()
		require.Error(t, err)
	})

	t.Run("rejects_unknown_type", func(t *testing.T) {
		t.Parallel()
		cfg := schedulerconfig.Default()
		cfg.HistoryStorage = &schedulerconfig.HistoryStorageConfig{Type: "mongo"}
		_, err := factory.New(&cfg).UseClickHouseConn(&chConn{}).Build()
		require.Error(t, err)
	})

	t.Run("rejects_invalid_table", func(t *testing.T) {
		t.Parallel()
		_, err := factory.New(clickHouseHistoryConfig(&schedulerconfig.HistoryClickHouseConfig{TableName: "db.h", Engine: "MergeTree"})).
			UseClickHouseConn(&chConn{}).Build()
		require.ErrorIs(t, err, chstorage.ErrInvalidIdentifier)
	})

	t.Run("builds_without_io", func(t *testing.T) {
		t.Parallel()
		conn := &chConn{}
		s, err := factory.New(clickHouseHistoryConfig(&schedulerconfig.HistoryClickHouseConfig{TableName: "h", Engine: "MergeTree"})).
			UseClickHouseConn(conn).Build()
		require.NoError(t, err)
		require.NotNil(t, s)
		require.Empty(t, conn.execs)
	})

	t.Run("ensure_schema_defaults_ttl_to_retention", func(t *testing.T) {
		t.Parallel()
		conn := &chConn{}
		cfg := clickHouseHistoryConfig(&schedulerconfig.HistoryClickHouseConfig{TableName: "h", Engine: "MergeTree", EnsureSchema: true})
		cfg.HistoryRetention = 48 * time.Hour
		_, err := factory.New(cfg).UseClickHouseConn(conn).Build()
		require.NoError(t, err)
		want, err := chstorage.SchemaDDL("h", "MergeTree", "", 48*time.Hour)
		require.NoError(t, err)
		require.Equal(t, []string{want}, conn.execs)
	})

	t.Run("ensure_schema_explicit_ttl", func(t *testing.T) {
		t.Parallel()
		conn := &chConn{}
		_, err := factory.New(clickHouseHistoryConfig(&schedulerconfig.HistoryClickHouseConfig{
			TableName: "h", Engine: "MergeTree", TTL: time.Hour, EnsureSchema: true,
		})).UseClickHouseConn(conn).Build()
		require.NoError(t, err)
		want, err := chstorage.SchemaDDL("h", "MergeTree", "", time.Hour)
		require.NoError(t, err)
		require.Equal(t, []string{want}, conn.execs)
	})

	t.Run("ensure_schema_failure", func(t *testing.T) {
		t.Parallel()
		errDDL := errors.New("ddl failed")
		_, err := factory.New(clickHouseHistoryConfig(&schedulerconfig.HistoryClickHouseConfig{
			TableName: "h", Engine: "MergeTree", EnsureSchema: true,
		})).UseClickHouseConn(&chConn{err: errDDL}).Build()
		require.ErrorIs(t, err, errDDL)
	})
}
