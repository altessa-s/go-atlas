// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/messaging/broker"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/messaging/broker"
	"github.com/altessa-s/go-atlas/messaging/broker/msg"

	brokerconfig "github.com/altessa-s/go-atlas/config/broker"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	dataoutbox "github.com/altessa-s/go-atlas/reliability/outbox"
	outboxsql "github.com/altessa-s/go-atlas/reliability/outbox/storages/sqldb"
	schedtask "github.com/altessa-s/go-horai/task"
)

// capturingRegistrar records the scheduler tasks the builder registers.
type capturingRegistrar struct {
	mu         sync.Mutex
	registered []schedtask.Config
}

func (r *capturingRegistrar) Register(_ context.Context, cfg schedtask.Config) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.registered = append(r.registered, cfg)
	return nil
}

func (r *capturingRegistrar) ids() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.registered))
	for _, c := range r.registered {
		out = append(out, c.ID)
	}
	return out
}

// stubPublisher satisfies the outbox Publisher interface and fails every
// publish with err.
type stubPublisher struct{ err error }

func (p stubPublisher) Publish(context.Context, msg.Message) error { return p.err }

// outboxConfig returns a fully populated broker config, so a field the builder
// forgets to read shows up as a default rather than the configured value.
func outboxConfig() *brokerconfig.Config {
	return &brokerconfig.Config{
		Outbox: brokerconfig.Outbox{
			Enabled:                 true,
			FetchTimeout:            time.Second,
			HandleTimeout:           2 * time.Second,
			UpdateTimeout:           time.Second,
			MaxLockTime:             time.Minute,
			MessagesBatchSize:       10,
			RetryMaxAttempts:        3,
			RetryBaseDelay:          time.Second,
			RetryMaxDelay:           time.Minute,
			MaxPayloadBytes:         32,
			PublishedEventsLifetime: time.Hour,
			DefaultEventTTL:         time.Hour,
			DispatchSchedule:        "@every 2s",
			UnlockSchedule:          "@every 11s",
			CleanupSchedule:         "@every 10m",
			ExpireSchedule:          "@every 11s",
			StatsSchedule:           "@every 30s",
			DispatchTaskID:          "svc-a-dispatch",
			UnlockTaskID:            "svc-a-unlock",
			ExpireTaskID:            "svc-a-expire",
			CleanupTaskID:           "svc-a-cleanup",
			StatsTaskID:             "svc-a-stats",
		},
	}
}

// The builder and factory/reliability/outbox read the same brokerconfig.Outbox. A field one
// of them forgets is not a compile error and not a runtime error — the setting
// is simply ignored, and the operator's YAML quietly does nothing.
func TestCreateOutbox_RegistersEveryCycleUnderItsConfiguredID(t *testing.T) {
	t.Parallel()

	reg := &capturingRegistrar{}
	db, _ := testhelpers.NewFakeSQL(t, nil)
	ob, err := factory.New(outboxConfig()).
		UseScheduler(reg).
		CreateOutboxWithSQLDB(t.Context(), db, outboxsql.DialectPostgres, stubPublisher{})
	require.NoError(t, err)
	require.NotNil(t, ob)

	require.ElementsMatch(t,
		[]string{"svc-a-dispatch", "svc-a-unlock", "svc-a-expire", "svc-a-cleanup", "svc-a-stats"},
		reg.ids(),
		"every configured cycle must reach the scheduler under its configured ID")
}

// MaxPayloadBytes is the one outbox setting whose effect is observable without a
// store, so it stands in for the whole config→option conversion.
func TestCreateOutbox_AppliesThePayloadLimitFromConfig(t *testing.T) {
	t.Parallel()

	db, _ := testhelpers.NewFakeSQL(t, nil)
	ob, err := factory.New(outboxConfig()).CreateOutboxWithSQLDB(t.Context(), db, outboxsql.DialectPostgres, stubPublisher{})
	require.NoError(t, err)

	oversized := make([]byte, 64) // The config allows 32.
	require.ErrorIs(t,
		ob.Publish(t.Context(), *msg.NewMessage("billing.invoice.paid", oversized)),
		dataoutbox.ErrPayloadTooLarge)
}

// A disabled outbox builds into nothing rather than into a running one.
func TestCreateOutbox_DisabledReturnsNothing(t *testing.T) {
	t.Parallel()

	cfg := outboxConfig()
	cfg.Outbox.Enabled = false

	db, _ := testhelpers.NewFakeSQL(t, nil)
	ob, err := factory.New(cfg).CreateOutboxWithSQLDB(t.Context(), db, outboxsql.DialectPostgres, stubPublisher{})
	require.NoError(t, err)
	require.Nil(t, ob)
}

// A missing publisher is a dependency error, not a nil outbox that fails later.
func TestCreateOutbox_RequiresAPublisher(t *testing.T) {
	t.Parallel()

	db, _ := testhelpers.NewFakeSQL(t, nil)
	_, err := factory.New(outboxConfig()).CreateOutboxWithSQLDB(t.Context(), db, outboxsql.DialectPostgres, nil)
	require.Error(t, err)
}

// The factory-built outbox must keep the broker retry classification: a closed
// NATS connection never reopens, so the event is dead-lettered at once instead
// of spending its retry budget.
func TestCreateOutbox_ClosedConnectionIsRejected(t *testing.T) {
	t.Parallel()

	created := time.Now().UTC()
	db, fake := testhelpers.NewFakeSQL(t, func(query string, _ []any) testhelpers.FakeSQLReply {
		if strings.HasPrefix(strings.TrimSpace(query), "SELECT") {
			return testhelpers.FakeSQLReply{
				Columns: []string{"id", "topic", "payload", "attempts", "last_error", "created_at", "locked_on", "published_at", "expires_at", "seq"},
				Rows: [][]driver.Value{{
					[]byte("event-1"), []byte("billing.invoice.paid"), []byte(`{"d":"e30="}`),
					int64(0), nil, created, nil, nil, created, int64(1),
				}},
			}
		}
		return testhelpers.FakeSQLReply{Affected: 1}
	})

	ob, err := factory.New(outboxConfig()).
		CreateOutboxWithSQLDB(t.Context(), db, outboxsql.DialectPostgres, stubPublisher{err: broker.ErrConnectionClosed})
	require.NoError(t, err)
	require.NoError(t, ob.RunDispatchCycle(t.Context()))

	statuses := make([]any, 0)
	for _, c := range fake.Calls() {
		if strings.HasPrefix(strings.TrimSpace(c.Query), "UPDATE") && len(c.Args) > 0 {
			statuses = append(statuses, c.Args[0])
		}
	}
	require.True(t, slices.Contains(statuses, any(string(dataoutbox.StatusRejected))),
		"a closed connection must dead-letter the event, got statuses %v", statuses)
}

func TestCreateOutboxWithSQLDB(t *testing.T) {
	t.Parallel()

	_, err := factory.New(outboxConfig()).CreateOutboxWithSQLDB(t.Context(), nil, outboxsql.DialectPostgres, stubPublisher{})
	require.ErrorIs(t, err, corefactory.ErrDependencyRequired)

	db, fake := testhelpers.NewFakeSQL(t, nil)
	ob, err := factory.New(outboxConfig()).CreateOutboxWithSQLDB(t.Context(), db, outboxsql.DialectPostgres, stubPublisher{})
	require.NoError(t, err)
	require.NotNil(t, ob)
	require.Empty(t, fake.Calls(), "without ensureSchema the build performs no I/O")
}

func TestCreateOutboxWithSQLDBEnsureSchema(t *testing.T) {
	t.Parallel()

	t.Run("creates_schema", func(t *testing.T) {
		t.Parallel()
		cfg := outboxConfig()
		cfg.Outbox.EnsureSchema = true
		db, fake := testhelpers.NewFakeSQL(t, nil)
		ob, err := factory.New(cfg).CreateOutboxWithSQLDB(t.Context(), db, outboxsql.DialectMySQL, stubPublisher{})
		require.NoError(t, err)
		require.NotNil(t, ob)
		calls := fake.Calls()
		require.NotEmpty(t, calls)
		require.Contains(t, calls[0].Query, "CREATE TABLE IF NOT EXISTS")
	})

	t.Run("disabled_outbox_runs_no_ddl", func(t *testing.T) {
		t.Parallel()
		cfg := outboxConfig()
		cfg.Outbox.Enabled, cfg.Outbox.EnsureSchema = false, true
		db, fake := testhelpers.NewFakeSQL(t, nil)
		ob, err := factory.New(cfg).CreateOutboxWithSQLDB(t.Context(), db, outboxsql.DialectMySQL, stubPublisher{})
		require.NoError(t, err)
		require.Nil(t, ob)
		require.Empty(t, fake.Calls())
	})

	t.Run("propagates_failure", func(t *testing.T) {
		t.Parallel()
		cfg := outboxConfig()
		cfg.Outbox.EnsureSchema = true
		boom := errors.New("ddl denied")
		db, _ := testhelpers.NewFakeSQL(t, func(string, []any) testhelpers.FakeSQLReply { return testhelpers.FakeSQLReply{Err: boom} })
		_, err := factory.New(cfg).CreateOutboxWithSQLDB(t.Context(), db, outboxsql.DialectPostgres, stubPublisher{})
		require.ErrorIs(t, err, boom)
	})
}
