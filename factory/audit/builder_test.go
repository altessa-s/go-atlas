// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-aip/pagination/pagetoken"

	"github.com/altessa-s/go-atlas-core/types/redacted"
	"github.com/altessa-s/go-atlas/audit"
	"github.com/altessa-s/go-atlas/audit/storages/memory"
	"github.com/altessa-s/go-atlas/factory/audit"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	coreruntime "github.com/altessa-s/go-atlas-core/runtime"
	auditconfig "github.com/altessa-s/go-atlas/config/audit"
	dispatchconfig "github.com/altessa-s/go-atlas/config/dispatch"
	pagingconfig "github.com/altessa-s/go-atlas/config/paging"
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

	_, err := factory.New(nil).Build(t.Context())
	require.Error(t, err)
}

func TestBuild_DisabledReturnsSentinel(t *testing.T) {
	t.Parallel()

	cfg := memoryConfig()
	cfg.Enabled = false

	_, err := factory.New(cfg).Build(t.Context())
	require.ErrorIs(t, err, factory.ErrDisabled)
}

// The regression this whole mapping exists for: before it, `storage` was
// described, validated and documented while the builder demanded an injected
// dispatcher, so a configured backend produced neither an effect nor an error.
func TestBuild_CreatesDispatcherFromConfig(t *testing.T) {
	t.Parallel()

	var hooks coreruntime.HookGroup

	auditor, err := factory.New(memoryConfig()).UseShutdownHooks(&hooks).Build(t.Context())
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

	auditor, err := factory.New(memoryConfig()).UseDispatcher(stub).Build(t.Context())
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

	_, err := factory.New(cfg).Build(t.Context())
	require.Error(t, err)
}

// A mongo type without a registered builder must fail explicitly rather than
// fall back to memory.
func TestBuild_UnregisteredMongoFails(t *testing.T) {
	t.Parallel()

	cfg := memoryConfig()
	cfg.Storage = auditconfig.Storage{
		Type:  auditconfig.StorageTypeMongo,
		Mongo: &auditconfig.StorageMongo{CollectionName: "audit_events"},
	}

	_, err := factory.New(cfg).Build(t.Context())
	require.Error(t, err)
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

	auditor, err := factory.New(cfg).UseShutdownHooks(&hooks).Build(t.Context())
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

	_, err := factory.New(memoryConfig()).UseShutdownHooks(&hooks).Build(t.Context())
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
		Build(t.Context())
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

func TestBuildStorage_UnregisteredClickHouseFails(t *testing.T) {
	t.Parallel()

	cfg := auditconfig.Config{
		Enabled: true,
		Storage: auditconfig.Storage{
			Type:       auditconfig.StorageTypeClickHouse,
			ClickHouse: &auditconfig.StorageClickHouse{TableName: "audit_events", Engine: "ReplacingMergeTree", MaxBatchSize: 1000},
		},
	}
	_, err := factory.New(&cfg).BuildStorage(t.Context())
	require.Error(t, err)
}

var errStorage = errors.New("storage failed")

func TestUseStorage(t *testing.T) {
	t.Parallel()

	for _, typ := range []auditconfig.StorageType{auditconfig.StorageTypeMongo, auditconfig.StorageTypeClickHouse} {
		t.Run(string(typ), func(t *testing.T) {
			t.Parallel()
			cfg := memoryConfig()
			cfg.Storage.Type = typ

			_, err := factory.New(cfg).UseStorage(typ, nil).BuildStorage(t.Context())
			require.Error(t, err)

			failing := func(context.Context, *auditconfig.Config, *slog.Logger) (audit.Storage, error) {
				return nil, errStorage
			}
			_, err = factory.New(cfg).UseStorage(typ, failing).BuildStorage(t.Context())
			require.ErrorIs(t, err, errStorage)

			want := memory.New()
			registered := func(_ context.Context, got *auditconfig.Config, logger *slog.Logger) (audit.Storage, error) {
				require.Same(t, cfg, got)
				require.NotNil(t, logger)
				return want, nil
			}
			storage, err := factory.New(cfg).UseStorage(typ, failing).UseStorage(typ, registered).BuildStorage(t.Context())
			require.NoError(t, err, "a later registration replaces the earlier one")
			require.Same(t, want, storage)

			var hooks coreruntime.HookGroup
			auditor, err := factory.New(cfg).
				UseStorage(typ, registered).
				UseShutdownHooks(&hooks).
				Build(t.Context())
			require.NoError(t, err)
			require.NotNil(t, auditor)
			require.NoError(t, hooks.Shutdown(t.Context()))
		})
	}
}

func TestBuildPageTokens(t *testing.T) {
	t.Parallel()

	tokens, err := factory.New(&auditconfig.Config{}).BuildPageTokens()
	require.NoError(t, err)
	require.Nil(t, tokens, "no paging block disables page tokens")

	key := redacted.RedactedString(strings.Repeat("k", 32))
	cfg := auditconfig.Config{Paging: &pagingconfig.Config{SigningKey: key, TokenTTL: time.Hour}}
	tokens, err = factory.New(&cfg).BuildPageTokens()
	require.NoError(t, err)

	storage := memory.New()
	require.NoError(t, storage.StoreBatch(t.Context(), []*audit.Event{
		{ID: "a", Timestamp: time.UnixMilli(1)}, {ID: "b", Timestamp: time.UnixMilli(2)},
	}))
	page, err := audit.FetchPage(t.Context(), storage, tokens, audit.Query{Limit: 1})
	require.NoError(t, err)
	require.NotEmpty(t, page.Next)

	weak := auditconfig.Config{Paging: &pagingconfig.Config{SigningKey: "short"}}
	_, err = factory.New(&weak).BuildPageTokens()
	require.ErrorIs(t, err, pagetoken.ErrWeakKey)
	require.Error(t, weak.Validate(), "the schema rejects a short signing key too")
}

func sqlConfig(ensureSchema bool) *auditconfig.Config {
	return &auditconfig.Config{
		Enabled: true,
		Storage: auditconfig.Storage{
			Type: auditconfig.StorageTypeSQL,
			SQL: &auditconfig.StorageSQL{Dialect: auditconfig.SQLDialectMySQL, TableName: "app.audit", MaxBatchRows: 100,
				EnsureSchema: ensureSchema},
		},
	}
}

func TestBuildStorage_SQLRequiresDatabase(t *testing.T) {
	t.Parallel()
	_, err := factory.New(sqlConfig(false)).BuildStorage(t.Context())
	require.ErrorIs(t, err, factory.ErrSQLDBRequired)
}

func TestBuildStorage_SQL(t *testing.T) {
	t.Parallel()
	for _, ensure := range []bool{false, true} {
		t.Run(map[bool]string{false: "no_schema", true: "ensure_schema"}[ensure], func(t *testing.T) {
			t.Parallel()
			db, fake := testhelpers.NewFakeSQL(t, nil)
			storage, err := factory.New(sqlConfig(ensure)).UseSQLDB(db).BuildStorage(t.Context())
			require.NoError(t, err)
			require.NotNil(t, storage)
			var ddl int
			for _, c := range fake.Calls() {
				if strings.Contains(c.Query, "CREATE TABLE IF NOT EXISTS `app`.`audit`") {
					ddl++
				}
			}
			require.Equal(t, map[bool]int{false: 0, true: 1}[ensure], ddl)
		})
	}
}

// The caller's context bounds the startup schema creation: a canceled startup
// must not run DDL on a detached context.
func TestBuildStorage_SQLHonorsContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	db, fake := testhelpers.NewFakeSQL(t, nil)
	_, err := factory.New(sqlConfig(true)).UseSQLDB(db).BuildStorage(ctx)
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, fake.Calls())
}
