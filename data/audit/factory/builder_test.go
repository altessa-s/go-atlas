// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/types/redacted"
	"github.com/altessa-s/go-atlas/data/audit"
	"github.com/altessa-s/go-atlas/data/audit/factory"
	"github.com/altessa-s/go-atlas/data/audit/storages/memory"
	"github.com/altessa-s/go-atlas/data/keyset"

	auditconfig "github.com/altessa-s/go-atlas/config/audit"
	dispatchconfig "github.com/altessa-s/go-atlas/config/dispatch"
	coreruntime "github.com/altessa-s/go-atlas/core/runtime"
	auditclickhouse "github.com/altessa-s/go-atlas/data/audit/storages/clickhouse"
)

// stubDispatcher stands in for an externally-owned dispatcher so a test can
// tell "the builder used what I gave it" from "the builder built its own".
type stubDispatcher struct{ submitted int }

func (s *stubDispatcher) Submit(*audit.Event) bool { s.submitted++; return true }
func (s *stubDispatcher) Dropped() int64           { return 0 }

func memoryConfig() *auditconfig.Config {
	return &auditconfig.Config{
		Enabled: true,
		Storage: auditconfig.Storage{Type: auditconfig.StorageTypeMemory},
		Dispatch: dispatchconfig.Config{
			BufferSize:    16,
			BatchSize:     4,
			FlushInterval: 10 * time.Millisecond,
			Workers:       1,
			RetryAttempts: 1,
		},
		ShutdownTimeout: 5 * time.Second,
	}
}

func TestBuild_RequiresConfig(t *testing.T) {
	t.Parallel()

	_, err := factory.New(nil).Build()
	require.Error(t, err)
}

func TestBuild_DisabledReturnsSentinel(t *testing.T) {
	t.Parallel()

	cfg := memoryConfig()
	cfg.Enabled = false

	_, err := factory.New(cfg).Build()
	require.ErrorIs(t, err, factory.ErrDisabled)
}

// The regression this whole mapping exists for: before it, `storage` was
// described, validated and documented while the builder demanded an injected
// dispatcher, so a configured backend produced neither an effect nor an error.
func TestBuild_CreatesDispatcherFromConfig(t *testing.T) {
	t.Parallel()

	var hooks coreruntime.HookGroup

	auditor, err := factory.New(memoryConfig()).UseShutdownHooks(&hooks).Build()
	require.NoError(t, err)
	require.NotNil(t, auditor)

	require.True(t, auditor.Emit(&audit.Event{
		Type:   audit.EventTypeBusinessEvent,
		Action: audit.ActionCreate,
		Actor:  audit.Actor{Type: audit.ActorTypeUser, ID: "user-1"},
		Result: audit.Result{Status: audit.ResultStatusSuccess},
	}))

	require.NoError(t, hooks.Shutdown(t.Context()))
}

func TestBuild_InjectedDispatcherWins(t *testing.T) {
	t.Parallel()

	stub := &stubDispatcher{}

	auditor, err := factory.New(memoryConfig()).UseDispatcher(stub).Build()
	require.NoError(t, err)

	require.True(t, auditor.Emit(&audit.Event{
		Type:   audit.EventTypeBusinessEvent,
		Action: audit.ActionCreate,
		Actor:  audit.Actor{Type: audit.ActorTypeUser, ID: "user-1"},
		Result: audit.Result{Status: audit.ResultStatusSuccess},
	}))
	require.Equal(t, 1, stub.submitted, "the injected dispatcher must receive the event")

	require.NoError(t, auditor.Shutdown(t.Context()))
}

func TestBuild_RejectsUnknownStorageType(t *testing.T) {
	t.Parallel()

	cfg := memoryConfig()
	cfg.Storage.Type = "elasticsearch"

	_, err := factory.New(cfg).Build()
	require.Error(t, err)
}

// Mongo storage needs a database; without one the failure must be explicit
// rather than a silent fallback to memory.
func TestBuild_MongoStorageRequiresDatabase(t *testing.T) {
	t.Parallel()

	cfg := memoryConfig()
	cfg.Storage = auditconfig.Storage{
		Type:  auditconfig.StorageTypeMongo,
		Mongo: &auditconfig.StorageMongo{CollectionName: "audit_events"},
	}

	_, err := factory.New(cfg).Build()
	require.ErrorIs(t, err, factory.ErrMongoDatabaseRequired)
}

// WAL settings live under `dispatch`; the engine must actually receive them,
// which is observable through the segment directory being populated.
func TestBuild_HonorsDispatchWAL(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "wal")

	cfg := memoryConfig()
	cfg.Dispatch.WAL = &dispatchconfig.WAL{
		Enabled:         true,
		Dir:             dir,
		MaxSegmentBytes: 1 << 20,
		MaxBytes:        1 << 22,
		FsyncInterval:   5 * time.Millisecond,
	}

	var hooks coreruntime.HookGroup

	auditor, err := factory.New(cfg).UseShutdownHooks(&hooks).Build()
	require.NoError(t, err)

	require.True(t, auditor.Emit(&audit.Event{
		Type:   audit.EventTypeBusinessEvent,
		Action: audit.ActionCreate,
		Actor:  audit.Actor{Type: audit.ActorTypeUser, ID: "user-wal"},
		Result: audit.Result{Status: audit.ResultStatusSuccess},
	}))

	require.NoError(t, hooks.Shutdown(t.Context()))
	require.DirExists(t, dir, "WAL config must reach the engine")
}

// A builder-owned engine has no other owner, so the builder must register its
// shutdown — otherwise the workers outlive the subsystem.
func TestBuild_RegistersEngineShutdownInScope(t *testing.T) {
	t.Parallel()

	var hooks coreruntime.HookGroup

	_, err := factory.New(memoryConfig()).UseShutdownHooks(&hooks).Build()
	require.NoError(t, err)

	// Engine shutdown plus the auditor's own hook.
	require.NoError(t, hooks.Shutdown(t.Context()))
}

// An injected dispatcher belongs to the caller; the builder must not schedule
// a shutdown for something it does not own.
func TestBuild_InjectedDispatcherIsNotShutDownByBuilder(t *testing.T) {
	t.Parallel()

	var hooks coreruntime.HookGroup

	auditor, err := factory.New(memoryConfig()).
		UseDispatcher(&stubDispatcher{}).
		UseShutdownHooks(&hooks).
		Build()
	require.NoError(t, err)

	require.NoError(t, hooks.Shutdown(t.Context()))

	// Only the auditor was in the scope, and stopping it stops emission.
	require.False(t, auditor.Emit(&audit.Event{
		Type:   audit.EventTypeBusinessEvent,
		Action: audit.ActionCreate,
		Actor:  audit.Actor{Type: audit.ActorTypeUser, ID: "user-1"},
		Result: audit.Result{Status: audit.ResultStatusSuccess},
	}))
}

func TestBuild_ClickHouseRequiresConn(t *testing.T) {
	t.Parallel()

	cfg := auditconfig.Config{
		Enabled: true,
		Storage: auditconfig.Storage{
			Type:       auditconfig.StorageTypeClickHouse,
			ClickHouse: &auditconfig.StorageClickHouse{TableName: "audit_events", Engine: "ReplacingMergeTree", MaxBatchSize: 1000},
		},
	}
	_, err := factory.New(&cfg).BuildStorage()
	require.ErrorIs(t, err, factory.ErrClickHouseConnRequired)
}

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

func TestBuildStorage_ClickHouseSchemaCheckIsBounded(t *testing.T) {
	t.Parallel()

	conn := &deadlineConn{}
	_, err := factory.New(clickHouseConfig()).UseClickHouseConn(conn).BuildStorage()

	require.ErrorIs(t, err, errSchemaQuery)
	require.True(t, conn.deadline, "the startup schema check runs under a deadline")
	require.InDelta(t, time.Minute, conn.remaining, float64(5*time.Second))
}

func TestBuildStorage_ClickHouseRejectsUnsafeTableName(t *testing.T) {
	t.Parallel()

	cfg := clickHouseConfig()
	cfg.Storage.ClickHouse.TableName = "audit`; DROP TABLE audit_events; --"
	_, err := factory.New(cfg).UseClickHouseConn(&deadlineConn{}).BuildStorage()

	require.ErrorIs(t, err, auditclickhouse.ErrInvalidIdentifier)
}

func TestBuildPageTokens(t *testing.T) {
	t.Parallel()

	tokens, err := factory.New(&auditconfig.Config{}).BuildPageTokens()
	require.NoError(t, err)
	require.Nil(t, tokens, "no paging block disables page tokens")

	key := redacted.RedactedString(strings.Repeat("k", 32))
	cfg := auditconfig.Config{Paging: &auditconfig.Paging{SigningKey: key, TokenTTL: time.Hour}}
	tokens, err = factory.New(&cfg).BuildPageTokens()
	require.NoError(t, err)

	storage := memory.New()
	require.NoError(t, storage.StoreBatch(t.Context(), []*audit.Event{
		{ID: "a", Timestamp: time.UnixMilli(1)}, {ID: "b", Timestamp: time.UnixMilli(2)},
	}))
	page, err := audit.FetchPage(t.Context(), storage, tokens, audit.Query{Limit: 1})
	require.NoError(t, err)
	require.NotEmpty(t, page.Next)

	weak := auditconfig.Config{Paging: &auditconfig.Paging{SigningKey: "short"}}
	_, err = factory.New(&weak).BuildPageTokens()
	require.ErrorIs(t, err, keyset.ErrWeakKey)
	require.Error(t, weak.Validate(), "the schema rejects a short signing key too")
}
