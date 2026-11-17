// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxit_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/tests/integration/outboxit"

	coreruntime "github.com/altessa-s/go-atlas-core/runtime"
	brokerconfig "github.com/altessa-s/go-atlas/config/broker"
	outboxfactory "github.com/altessa-s/go-atlas/factory/reliability/outbox"
)

// The factory wires the notify saver, listener and watcher together. The
// registrar records tasks without running them, so delivery requires a notification.
func TestWatch_SQLFactoryNotifyDeliversWithoutAPollCycle(t *testing.T) {
	t.Parallel()
	f := &fixture{sql: newSQLFixtureWithoutSchema(t, sqlSpecs()[0]), recorder: outboxit.NewRecorder()}
	cfg := brokerconfig.Default().Outbox
	cfg.Storage = &brokerconfig.OutboxStorageConfig{
		Type: brokerconfig.OutboxStorageTypeSQL,
		SQL: &brokerconfig.OutboxSQLStorageConfig{
			Dialect: brokerconfig.SQLDialectPostgres, Table: f.sql.events, EnsureSchema: true,
		},
	}
	cfg.Watch = brokerconfig.OutboxWatchNotify
	cfg.DispatchSchedule = "@every 24h"
	reg := &testhelpers.MockTaskRegistrar{}
	var hooks coreruntime.HookGroup
	t.Cleanup(func() { require.NoError(t, hooks.Shutdown(context.WithoutCancel(t.Context()))) })
	ob, err := outboxfactory.New(&cfg).UseSQLDB(f.sql.db).
		UseSQLListener(outboxit.PGListener{DSN: sqlSpecs()[0].DSN()}).
		UseScheduler(reg).UseShutdownHooks(&hooks).Build(t.Context(), f.recorder.Handler())
	require.NoError(t, err)
	require.NotNil(t, ob)
	dispatch, ok := reg.Task(cfg.DispatchTaskID)
	require.True(t, ok)
	require.Equal(t, "@every 24h", dispatch.Schedule)
	f.awaitWatcher(t, ob)

	saved := f.save(t, ob, event("billing.invoice.paid", `{"invoice":1}`))
	require.Eventually(t, func() bool { return f.recorder.Count() == 1 }, settleWindow, samplingInterval,
		"the factory watcher never delivered the saved event:\n%s", f.recorder)
	require.Equal(t, []string{`{"invoice":1}`}, f.recorder.Payloads())
	f.awaitSent(t, saved[0].Id)
	require.NoError(t, hooks.Shutdown(t.Context()))
}
