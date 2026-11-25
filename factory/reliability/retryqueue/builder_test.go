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
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/reliability/retryqueue"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	coreruntime "github.com/altessa-s/go-atlas-core/runtime"
	retryqueueconfig "github.com/altessa-s/go-atlas/config/retryqueue"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	retryqueuefactory "github.com/altessa-s/go-atlas/factory/reliability/retryqueue"
	memorystore "github.com/altessa-s/go-atlas/reliability/retryqueue/storages/memory"
	sqlstore "github.com/altessa-s/go-atlas/reliability/retryqueue/storages/sqldb"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

var errBuild = errors.New("build failed")

// requireValidationError asserts err carries the config's validation errors.
func requireValidationError(tb testing.TB, err error) {
	tb.Helper()
	_, ok := errors.AsType[validation.Errors](err)
	require.True(tb, ok, "want a config validation error, got %v", err)
}

type payload struct {
	Note string
}

func sqlConfig(dialect string, ensureSchema bool) retryqueueconfig.Config {
	return retryqueueconfig.Config{Storage: &retryqueueconfig.StorageConfig{
		Type: retryqueueconfig.StorageTypeSQL,
		SQL:  &retryqueueconfig.SQLStorageConfig{Dialect: dialect, Table: "app.retry_items", EnsureSchema: ensureSchema},
	}}
}

func mongoStorage() *retryqueueconfig.StorageConfig {
	return &retryqueueconfig.StorageConfig{
		Type:  retryqueueconfig.StorageTypeMongo,
		Mongo: &retryqueueconfig.MongoStorageConfig{Collection: "retry_items", FlagField: "flagged", FlagReasonField: "flagReason"},
	}
}

func TestBuildNilConfig(t *testing.T) {
	t.Parallel()
	_, err := retryqueuefactory.New[payload](nil).Build(t.Context())
	require.ErrorIs(t, err, corefactory.ErrConfigRequired)
}

func TestBuildInvalidConfig(t *testing.T) {
	t.Parallel()
	cfg := retryqueueconfig.Config{}
	_, err := retryqueuefactory.New[payload](&cfg).Build(t.Context())
	requireValidationError(t, err)

	cfg = retryqueueconfig.Config{Storage: &retryqueueconfig.StorageConfig{Type: "etcd"}}
	_, err = retryqueuefactory.New[payload](&cfg).Build(t.Context())
	requireValidationError(t, err)
}

func TestBuildMemory(t *testing.T) {
	t.Parallel()
	cfg := retryqueueconfig.Default()
	queue, err := retryqueuefactory.New[payload](&cfg).Build(t.Context())
	require.NoError(t, err)
	require.IsType(t, &memorystore.Store[payload]{}, queue)

	got, err := queue.Mark(t.Context(), retryqueue.Item[payload]{Key: "k", Payload: payload{Note: "n"}})
	require.NoError(t, err)
	require.Equal(t, retryqueue.StatePending, got.State)
}

func TestBuildMongoNotRegistered(t *testing.T) {
	t.Parallel()
	cfg := retryqueueconfig.Config{Storage: mongoStorage()}
	_, err := retryqueuefactory.New[payload](&cfg).Build(t.Context())
	require.ErrorIs(t, err, corefactory.ErrNotRegistered)
	require.ErrorContains(t, err, "factory/reliability/retryqueue/storages/mongo")
}

func TestUseStorage(t *testing.T) {
	t.Parallel()
	cfg := retryqueueconfig.Config{Storage: mongoStorage()}

	_, err := retryqueuefactory.New[payload](&cfg).UseStorage(retryqueueconfig.StorageTypeMongo, nil).Build(t.Context())
	require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)

	failing := func(context.Context, *retryqueueconfig.StorageConfig) (retryqueue.Storage[payload], error) {
		return nil, errBuild
	}
	for _, typ := range []retryqueueconfig.StorageType{retryqueueconfig.StorageTypeMemory, retryqueueconfig.StorageTypeSQL, ""} {
		_, err := retryqueuefactory.New[payload](&cfg).UseStorage(typ, failing).Build(t.Context())
		require.ErrorIs(t, err, corefactory.ErrUnsupported)
	}
	_, err = retryqueuefactory.New[payload](&cfg).UseStorage(retryqueueconfig.StorageTypeMongo, failing).Build(t.Context())
	require.ErrorIs(t, err, errBuild)

	var called bool
	want := memorystore.New[payload]()
	registered := func(ctx context.Context, got *retryqueueconfig.StorageConfig) (retryqueue.Storage[payload], error) {
		require.NotNil(t, ctx)
		require.Equal(t, cfg.Storage, got)
		require.NotSame(t, cfg.Storage, got)
		called = true
		return want, nil
	}
	queue, err := retryqueuefactory.New[payload](&cfg).
		UseStorage(retryqueueconfig.StorageTypeMongo, failing).
		UseStorage(retryqueueconfig.StorageTypeMongo, registered).
		Build(t.Context())
	require.NoError(t, err)
	require.True(t, called, "the later registration replaces the earlier one")
	require.Same(t, want, queue)
}

func TestBuildSQLRequiresDatabase(t *testing.T) {
	t.Parallel()
	cfg := sqlConfig(retryqueueconfig.SQLDialectPostgres, false)
	_, err := retryqueuefactory.New[payload](&cfg).Build(t.Context())
	require.ErrorIs(t, err, corefactory.ErrDependencyRequired)
}

func TestBuildDoesNotMutateConfig(t *testing.T) {
	t.Parallel()
	cfg := retryqueueconfig.Config{Storage: &retryqueueconfig.StorageConfig{Type: retryqueueconfig.StorageTypeSQL}}
	_, err := retryqueuefactory.New[payload](&cfg).Build(t.Context())
	require.Error(t, err)
	require.Nil(t, cfg.Storage.SQL)
}

func TestBuildSQLRejectsUnknownDialect(t *testing.T) {
	t.Parallel()
	db, _ := testhelpers.NewFakeSQL(t, nil)
	cfg := sqlConfig("oracle", false)
	_, err := retryqueuefactory.New[payload](&cfg).UseSQLDB(db).Build(t.Context())
	requireValidationError(t, err)
}

func TestBuildSQLRejectsInvalidTable(t *testing.T) {
	t.Parallel()
	db, _ := testhelpers.NewFakeSQL(t, nil)
	cfg := sqlConfig(retryqueueconfig.SQLDialectPostgres, false)
	cfg.Storage.SQL.Table = "bad name;"
	_, err := retryqueuefactory.New[payload](&cfg).UseSQLDB(db).Build(t.Context())
	require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
	require.ErrorIs(t, err, sqlstore.ErrInvalidTableName)
}

func TestBuildSQL(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		ensureSchema bool
	}{
		{name: "no schema"},
		{name: "ensure schema", ensureSchema: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db, fake := testhelpers.NewFakeSQL(t, nil)
			cfg := sqlConfig(retryqueueconfig.SQLDialectMySQL, tc.ensureSchema)

			queue, err := retryqueuefactory.New[payload](&cfg).UseSQLDB(db).Build(t.Context())
			require.NoError(t, err)
			require.IsType(t, &sqlstore.Store[payload]{}, queue)

			var ddl []string
			for _, c := range fake.Calls() {
				if strings.HasPrefix(c.Query, "CREATE TABLE") {
					ddl = append(ddl, c.Query)
				}
			}
			if !tc.ensureSchema {
				require.Empty(t, ddl)
				return
			}
			require.Len(t, ddl, 1)
			require.Contains(t, ddl[0], "`app`.`retry_items`")
		})
	}
}

// The configured retention and the injected codec reach the SQL store.
func TestBuildSQLWiresRetentionAndCodec(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, nil)
	cfg := sqlConfig(retryqueueconfig.SQLDialectPostgres, false)
	cfg.Storage.SQL.ResolvedRetention = time.Hour
	codec := &recordingCodec{}

	queue, err := retryqueuefactory.New[payload](&cfg).UseSQLDB(db).UseCodec(codec).Build(t.Context())
	require.NoError(t, err)
	store, ok := queue.(*sqlstore.Store[payload])
	require.True(t, ok)

	_, err = store.PurgeExpired(t.Context())
	require.NoError(t, err)
	calls := fake.Calls()
	require.Len(t, calls, 1)
	require.Equal(t, []any{time.Hour.Nanoseconds()}, calls[0].Args)

	_, err = store.Mark(t.Context(), retryqueue.Item[payload]{Key: "k", Payload: payload{Note: "n"}})
	require.NoError(t, err)
	require.Equal(t, 1, codec.serialized)
}

// The caller's context bounds the startup schema creation: a canceled startup
// must not run DDL on a detached context.
func TestBuildSQLHonorsContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	db, fake := testhelpers.NewFakeSQL(t, func(string, []any) testhelpers.FakeSQLReply {
		return testhelpers.FakeSQLReply{Rows: [][]driver.Value{}}
	})
	cfg := sqlConfig(retryqueueconfig.SQLDialectMySQL, true)
	queue, err := retryqueuefactory.New[payload](&cfg).UseSQLDB(db).Build(ctx)
	require.True(t, queue == nil, "no store escapes failed setup")
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, fake.Calls())
}

// recordingCodec is a JSON-like codec that counts its calls.
type recordingCodec struct {
	serialized int
}

func (c *recordingCodec) Serialize(any) ([]byte, error) {
	c.serialized++
	return []byte(`{}`), nil
}

func (c *recordingCodec) Deserialize([]byte, any) error { return nil }

// A purge interval reaches the SQL store, whose background purge starts with
// the first queue call and stops when the builder's shutdown hooks run.
func TestBuildSQLPurgeLoopStopsOnShutdown(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		entered, release, exited := make(chan struct{}), make(chan struct{}), make(chan struct{})
		db, _ := testhelpers.NewFakeSQL(t, func(query string, _ []any) testhelpers.FakeSQLReply {
			if strings.Contains(query, "resolved_at <=") {
				close(entered)
				<-release
				close(exited)
			}
			return testhelpers.FakeSQLReply{}
		})
		cfg := sqlConfig(retryqueueconfig.SQLDialectPostgres, false)
		cfg.Storage.SQL.ResolvedRetention = time.Hour
		cfg.Storage.SQL.PurgeInterval = time.Minute
		var hooks coreruntime.HookGroup
		queue, err := retryqueuefactory.New[payload](&cfg).UseSQLDB(db).UseShutdownHooks(&hooks).Build(t.Context())
		require.NoError(t, err)
		require.NoError(t, queue.Delete(t.Context(), "k"))
		<-entered
		ended, cancel := context.WithCancel(t.Context())
		cancel()
		stopped := make(chan error, 1)
		go func() { stopped <- hooks.Shutdown(ended) }()
		synctest.Wait()
		premature := len(stopped)
		close(release)
		require.NoError(t, <-stopped)
		require.Zero(t, premature, "shutdown returned while the purge was running")
		select {
		case <-exited:
		default:
			require.FailNow(t, "purge still running")
		}
		require.NoError(t, queue.(*sqlstore.Store[payload]).Close(ended), "worker exit is already acknowledged")
	})
}

// A purge interval without retention leaves the background loop inactive.
func TestBuildSQLPurgeIntervalWithoutRetention(t *testing.T) {
	t.Parallel()
	db, _ := testhelpers.NewFakeSQL(t, nil)
	cfg := sqlConfig(retryqueueconfig.SQLDialectPostgres, false)
	cfg.Storage.SQL.PurgeInterval = time.Minute
	_, err := retryqueuefactory.New[payload](&cfg).UseSQLDB(db).Build(t.Context())
	require.NoError(t, err)
}

func TestMemoryBuilderUsesPayloadCloner(t *testing.T) {
	t.Parallel()
	type privatePayload struct{ bytes []byte }
	cfg := retryqueueconfig.Default()
	queue, err := retryqueuefactory.New[privatePayload](&cfg).
		UseMemoryPayloadClone(func(p privatePayload) privatePayload { return privatePayload{bytes: slices.Clone(p.bytes)} }).
		Build(t.Context())
	require.NoError(t, err)
	bytes := []byte{1}
	value, err := queue.Mark(t.Context(), retryqueue.Item[privatePayload]{Key: "k", Payload: privatePayload{bytes: bytes}})
	require.NoError(t, err)
	bytes[0] = 2
	value.Payload.bytes[0] = 3
	due, err := queue.ListDue(t.Context(), time.Now(), 1)
	require.NoError(t, err)
	require.Len(t, due, 1)
	require.Equal(t, []byte{1}, due[0].Payload.bytes)
}

func TestBuildCustomBackend(t *testing.T) {
	t.Parallel()
	cfg := retryqueueconfig.Config{Storage: &retryqueueconfig.StorageConfig{Type: "custom"}}

	build := func(context.Context, *retryqueueconfig.StorageConfig) (retryqueue.Storage[payload], error) {
		return memorystore.New[payload](), nil
	}
	b := retryqueuefactory.New[payload](&cfg).UseStorage("custom", build)
	cfg.Storage.SetRegisteredTypes(b.RegisteredStorageTypes())
	require.NoError(t, cfg.Validate())
	_, err := retryqueuefactory.New[payload](&cfg).Build(t.Context())
	var fields validation.Errors
	require.ErrorAs(t, err, &fields)
	cfg.Storage.SetRegisteredTypes(nil)
	got, err := b.Build(t.Context())
	require.NoError(t, err)
	require.NotNil(t, got)
}
