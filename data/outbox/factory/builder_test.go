// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/data/outbox"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	outboxsql "github.com/altessa-s/go-atlas/data/outbox/store/sqldb"
)

// Use the internal assembly boundary to test registration without a database.
func TestBuildPropagatesRegistrationFailureAndRetainsRetryableOutbox(t *testing.T) {
	t.Parallel()
	boom := errors.New("registrar unavailable")
	reg := &testhelpers.MockTaskRegistrar{Err: boom}
	b := New(&config.Outbox{Enabled: true, DispatchSchedule: "@every 1m",
		DispatchTaskID: "dispatch", UnlockTaskID: "unlock", ExpireTaskID: "expire", CleanupTaskID: "cleanup", StatsTaskID: "stats",
	}).UseScheduler(reg)
	ob, err := b.createOutboxWithStore(nil, nil)
	require.ErrorIs(t, err, boom)
	require.NotNil(t, ob)
	reg.Err = nil
	require.NoError(t, ob.RegisterTasks(t.Context()))
}

func TestBuildWithSQLDB(t *testing.T) {
	t.Parallel()
	cfg := &config.Outbox{Enabled: true}

	_, err := New(cfg).BuildWithSQLDB(nil, outboxsql.DialectPostgres, nil)
	require.ErrorContains(t, err, "SQL database")

	db, fake := testhelpers.NewFakeSQL(t, nil)
	_, err = New(cfg).BuildWithSQLDB(db, "oracle", nil)
	require.ErrorIs(t, err, outboxsql.ErrUnsupportedDialect)

	ob, err := New(cfg).BuildWithSQLDB(db, outboxsql.DialectMySQL, func(context.Context, outbox.Event) error { return nil })
	require.NoError(t, err)
	require.NotNil(t, ob)
	require.NotEmpty(t, fake.Calls(), "the events table is created on build")
}
