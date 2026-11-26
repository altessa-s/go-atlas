// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-aip/pagination/pagetoken"
	"github.com/altessa-s/go-nomos/loader"

	"github.com/altessa-s/go-atlas-core/types/redacted"
	"github.com/altessa-s/go-atlas/audit"
	"github.com/altessa-s/go-atlas/audit/storages/memory"
	"github.com/altessa-s/go-atlas/audit/storages/sqldb"
	"github.com/altessa-s/go-atlas/factory/audit"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	coreruntime "github.com/altessa-s/go-atlas-core/runtime"
	auditconfig "github.com/altessa-s/go-atlas/config/audit"
	dispatchconfig "github.com/altessa-s/go-atlas/config/dispatch"
	pagingconfig "github.com/altessa-s/go-atlas/config/paging"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	validation "github.com/go-ozzo/ozzo-validation/v4"
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

func TestBuildStorage_SQLMapsTTL(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, func(string, []any) testhelpers.FakeSQLReply {
		return testhelpers.FakeSQLReply{}
	})
	cfg := sqlConfig(false)
	cfg.Storage.SQL.TTL = time.Hour
	cfg.Storage.SQL.PurgeInterval = 0
	cfg.Storage.SQL.PurgeBatchSize = 7
	storage, err := factory.New(cfg).UseSQLDB(db).BuildStorage(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storage.Close(context.WithoutCancel(t.Context()))) })
	s, ok := storage.(*sqldb.Storage)
	require.True(t, ok)
	before := time.Now().Add(-time.Hour).UnixMilli()
	n, err := s.PurgeExpired(t.Context())
	after := time.Now().Add(-time.Hour).UnixMilli()
	require.NoError(t, err)
	require.Zero(t, n)
	calls := fake.Calls()
	require.Len(t, calls, 1)
	require.Contains(t, calls[0].Query, "DELETE FROM `app`.`audit`")
	require.GreaterOrEqual(t, calls[0].Args[0].(int64), before)
	require.LessOrEqual(t, calls[0].Args[0].(int64), after)
	require.Equal(t, int64(7), calls[0].Args[1])
}

type closeTrackingStorage struct {
	audit.Storage
	closed bool
}

func (s *closeTrackingStorage) Close(context.Context) error {
	s.closed = true
	return nil
}

func TestBuildClosesOwnedStorage(t *testing.T) {
	t.Parallel()
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "shutdown", true: "failed engine startup"}[fail], func(t *testing.T) {
			t.Parallel()
			cfg := memoryConfig()
			cfg.Storage.Type = auditconfig.StorageTypeMongo
			if fail {
				// A file cannot serve as a WAL directory, so startup fails after
				// creating the storage, without any filesystem permission reliance.
				path := filepath.Join(t.TempDir(), "file")
				require.NoError(t, os.WriteFile(path, nil, 0o600))
				cfg.Dispatch.WAL = &dispatchconfig.WAL{Enabled: true, Dir: path}
			}
			storage := &closeTrackingStorage{Storage: memory.New()}
			var hooks coreruntime.HookGroup
			builder := factory.New(cfg).UseShutdownHooks(&hooks).UseStorage(auditconfig.StorageTypeMongo,
				func(context.Context, *auditconfig.Config, *slog.Logger) (audit.Storage, error) { return storage, nil })
			_, err := builder.Build(t.Context())
			if fail {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.False(t, storage.closed)
				require.NoError(t, hooks.Shutdown(t.Context()))
			}
			require.True(t, storage.closed)
		})
	}
}

func TestBuildStorage_SQLPurgeShutdown(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, nil)
	var hooks coreruntime.HookGroup
	cfg := sqlConfig(false)
	cfg.Storage.SQL.TTL = time.Hour
	cfg.Storage.SQL.PurgeInterval = time.Millisecond
	cfg.Storage.SQL.PurgeBatchSize = 7
	storage, err := factory.New(cfg).UseSQLDB(db).UseShutdownHooks(&hooks).BuildStorage(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, hooks.Shutdown(context.WithoutCancel(t.Context()))) })
	require.NoError(t, storage.Store(t.Context(), &audit.Event{ID: "event", Timestamp: time.Now()}))
	testhelpers.WaitFor(t, time.Second, func() bool {
		for _, call := range fake.Calls() {
			if strings.HasPrefix(call.Query, "DELETE") {
				require.Equal(t, int64(7), call.Args[1])
				return true
			}
		}
		return false
	}, "configured interval starts retention")
	require.NoError(t, hooks.Shutdown(t.Context()))
	before := len(fake.Calls())
	time.Sleep(10 * time.Millisecond)
	require.Len(t, fake.Calls(), before, "shutdown stops the BuildStorage loop")
	require.NoError(t, db.PingContext(t.Context()))
}

func TestBuildCustomBackend(t *testing.T) {
	t.Parallel()
	cfg := memoryConfig()
	cfg.Storage.Type = "custom"
	var hooks coreruntime.HookGroup
	t.Cleanup(func() { require.NoError(t, hooks.Shutdown(context.WithoutCancel(t.Context()))) })
	build := func(context.Context, *auditconfig.Config, *slog.Logger) (audit.Storage, error) {
		return memory.New(), nil
	}
	b := factory.New(cfg).UseStorage("custom", build)
	cfg.Storage.SetRegisteredTypes(b.RegisteredStorageTypes())
	require.NoError(t, cfg.Storage.Validate())
	got, err := b.UseShutdownHooks(&hooks).Build(t.Context())
	require.NoError(t, err)
	require.NotNil(t, got)
	_, err = factory.New(cfg).UseShutdownHooks(&hooks).Build(t.Context())
	require.ErrorIs(t, err, corefactory.ErrUnsupported)
}

func newWarnLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})), &buf
}

func TestBuild_DefaultStorageWarnsAndUsesBoundedMemory(t *testing.T) {
	t.Parallel()

	t.Run("omitted_in_struct_warns_and_bounds", func(t *testing.T) {
		t.Parallel()

		logger, logs := newWarnLogger()
		var hooks coreruntime.HookGroup
		cfg := &auditconfig.Config{
			Enabled: true,
			Dispatch: dispatchconfig.Config{
				BufferSize:    16,
				BatchSize:     4,
				FlushInterval: 10 * time.Millisecond,
				Workers:       1,
				RetryAttempts: 1,
			},
			ShutdownTimeout: 5 * time.Second,
		}

		auditor, err := factory.New(cfg).UseLogger(logger).UseShutdownHooks(&hooks).Build(t.Context())
		require.NoError(t, err)
		require.NotNil(t, auditor)
		require.Contains(t, logs.String(), "level=WARN")
		require.Contains(t, logs.String(), "memory")
		require.NoError(t, hooks.Shutdown(t.Context()))

		logs.Reset()
		storage, err := factory.New(cfg).UseLogger(logger).BuildStorage(t.Context())
		require.NoError(t, err)
		mem, ok := storage.(*memory.Storage)
		require.True(t, ok)
		require.Equal(t, memory.DefaultMaxEvents, mem.Cap())
		require.Contains(t, logs.String(), "level=WARN")
	})

	t.Run("omitted_in_yaml_loader_warns", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "audit.yaml")
		require.NoError(t, os.WriteFile(path, []byte("audit:\n  enabled: true\n"), 0o600))

		var root struct {
			Audit auditconfig.Config `yaml:"audit"`
		}
		_, err := loader.New(nil, loader.WithPath(path), loader.WithSkipEnv()).Load(&root)
		require.NoError(t, err)

		logger, logs := newWarnLogger()
		var hooks coreruntime.HookGroup
		auditor, err := factory.New(&root.Audit).UseLogger(logger).UseShutdownHooks(&hooks).Build(t.Context())
		require.NoError(t, err)
		require.NotNil(t, auditor)
		require.Contains(t, logs.String(), "level=WARN")
		require.Contains(t, logs.String(), "memory")
		require.NoError(t, hooks.Shutdown(t.Context()))
	})

	t.Run("explicit_memory_does_not_warn_and_honors_max_events", func(t *testing.T) {
		t.Parallel()

		logger, logs := newWarnLogger()
		cfg := memoryConfig()
		cfg.Storage.Memory = &auditconfig.StorageMemory{MaxEvents: 3}

		storage, err := factory.New(cfg).UseLogger(logger).BuildStorage(t.Context())
		require.NoError(t, err)
		require.Empty(t, logs.String(), "explicit storage.type=memory must not emit default-storage warning")

		mem, ok := storage.(*memory.Storage)
		require.True(t, ok)
		require.Equal(t, 3, mem.Cap())

		for _, id := range []string{"e1", "e2", "e3", "e4"} {
			require.NoError(t, mem.Store(t.Context(), &audit.Event{ID: id, Timestamp: time.Now()}))
		}
		require.Equal(t, 3, mem.Len())
		require.Equal(t, "e2", mem.Events()[0].ID)
	})

	t.Run("invalid_memory_max_events_rejected", func(t *testing.T) {
		t.Parallel()

		for _, tc := range []struct {
			maxEvents int
			sentinel  error
		}{
			{maxEvents: 0, sentinel: validation.ErrRequired},
			{maxEvents: -5, sentinel: validation.ErrMinGreaterEqualThanRequired},
		} {
			cfg := memoryConfig()
			cfg.Storage.Memory = &auditconfig.StorageMemory{MaxEvents: tc.maxEvents}
			_, err := factory.New(cfg).BuildStorage(t.Context())
			require.ErrorIs(t, err, corefactory.ErrConfigRequired)
			require.ErrorIs(t, err, tc.sentinel)
		}
	})
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *syncBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
}

// TestBuild_DefaultStorageWarnsViaDefaultLoggerWhenUnset is serial because it
// temporarily replaces the process-wide slog.Default logger.
func TestBuild_DefaultStorageWarnsViaDefaultLoggerWhenUnset(t *testing.T) {
	logs := &syncBuffer{}
	original := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(original) })

	cfg := &auditconfig.Config{
		Enabled: true,
		Dispatch: dispatchconfig.Config{
			BufferSize:    16,
			BatchSize:     4,
			FlushInterval: 10 * time.Millisecond,
			Workers:       1,
			RetryAttempts: 1,
		},
		ShutdownTimeout: 5 * time.Second,
	}

	var hooks coreruntime.HookGroup
	auditor, err := factory.New(cfg).UseShutdownHooks(&hooks).Build(t.Context())
	require.NoError(t, err)
	require.NotNil(t, auditor)
	require.Contains(t, logs.String(), "level=WARN")
	require.Contains(t, logs.String(), "memory")
	require.NoError(t, hooks.Shutdown(t.Context()))

	logs.Reset()
	storage, err := factory.New(cfg).BuildStorage(t.Context())
	require.NoError(t, err)
	require.NotNil(t, storage)
	require.Contains(t, logs.String(), "level=WARN")
	require.Contains(t, logs.String(), "memory")

	// Explicit UseLogger (including DiscardHandler) must not leak to slog.Default.
	logs.Reset()
	_, err = factory.New(cfg).UseLogger(slog.New(slog.DiscardHandler)).BuildStorage(t.Context())
	require.NoError(t, err)
	require.Empty(t, logs.String())
}
