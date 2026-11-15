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

	"github.com/altessa-s/go-atlas/factory/audit/storages/clickhouse"

	auditclickhouse "github.com/altessa-s/go-atlas/audit/storages/clickhouse"
	auditconfig "github.com/altessa-s/go-atlas/config/audit"
	auditfactory "github.com/altessa-s/go-atlas/factory/audit"
)

var errSchemaQuery = errors.New("schema query failed")

// deadlineConn records whether the schema check ran under a deadline. The
// embedded interface covers the methods the check never calls.
type deadlineConn struct {
	auditclickhouse.Conn
	remaining time.Duration
	deadline  bool
}

func (c *deadlineConn) Query(ctx context.Context, _ string, _ ...any) (driver.Rows, error) {
	var deadline time.Time
	deadline, c.deadline = ctx.Deadline()
	c.remaining = time.Until(deadline)
	return nil, errSchemaQuery
}

func clickHouseConfig() *auditconfig.Config {
	return &auditconfig.Config{
		Enabled: true,
		Storage: auditconfig.Storage{
			Type: auditconfig.StorageTypeClickHouse,
			ClickHouse: &auditconfig.StorageClickHouse{
				TableName: "audit_events", Engine: "ReplacingMergeTree", MaxBatchSize: 1000, DDLTimeout: time.Minute,
			},
		},
	}
}

func buildStorage(t *testing.T, cfg *auditconfig.Config, conn auditclickhouse.Conn) error {
	t.Helper()
	_, err := auditfactory.New(cfg).
		UseStorage(auditconfig.StorageTypeClickHouse, factory.Builder(conn)).
		BuildStorage(t.Context())
	return err
}

func TestBuilder_RequiresConn(t *testing.T) {
	t.Parallel()

	require.ErrorIs(t, buildStorage(t, clickHouseConfig(), nil), factory.ErrConnRequired)
}

func TestBuilder_RequiresSection(t *testing.T) {
	t.Parallel()

	cfg := clickHouseConfig()
	cfg.Storage.ClickHouse = nil
	err := buildStorage(t, cfg, &deadlineConn{})
	require.Error(t, err)
	require.NotErrorIs(t, err, factory.ErrConnRequired)
}

func TestBuilder_SchemaCheckIsBounded(t *testing.T) {
	t.Parallel()

	conn := &deadlineConn{}
	cfg := clickHouseConfig()
	cfg.Dispatch.BatchSize = 10 // below the recommendation: warns, does not fail
	err := buildStorage(t, cfg, conn)

	require.ErrorIs(t, err, errSchemaQuery)
	require.True(t, conn.deadline, "the startup schema check runs under a deadline")
	require.InDelta(t, time.Minute, conn.remaining, float64(5*time.Second))
}

func TestBuilder_RejectsUnsafeTableName(t *testing.T) {
	t.Parallel()

	cfg := clickHouseConfig()
	cfg.Storage.ClickHouse.TableName = "audit`; DROP TABLE audit_events; --"

	require.ErrorIs(t, buildStorage(t, cfg, &deadlineConn{}), auditclickhouse.ErrInvalidIdentifier)
}
