// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
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
)

var errBuild = errors.New("build failed")

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
	require.ErrorContains(t, err, "validate retry queue config")

	cfg = retryqueueconfig.Config{Storage: &retryqueueconfig.StorageConfig{Type: "etcd"}}
	_, err = retryqueuefactory.New[payload](&cfg).Build(t.Context())
	require.ErrorContains(t, err, "validate retry queue config")
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
	for _, typ := range []retryqueueconfig.StorageType{retryqueueconfig.StorageTypeMemory, retryqueueconfig.StorageTypeSQL, "etcd"} {
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
	require.ErrorContains(t, err, "validate retry queue config")
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
	_, err := retryqueuefactory.New[payload](&cfg).UseSQLDB(db).Build(ctx)
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
	db, fake := testhelpers.NewFakeSQL(t, nil)
	cfg := sqlConfig(retryqueueconfig.SQLDialectPostgres, false)
	cfg.Storage.SQL.ResolvedRetention = time.Hour
	cfg.Storage.SQL.PurgeInterval = time.Millisecond
	cfg.Storage.SQL.PurgeBatchSize = 7
	var hooks coreruntime.HookGroup

	queue, err := retryqueuefactory.New[payload](&cfg).UseSQLDB(db).UseShutdownHooks(&hooks).Build(t.Context())
	require.NoError(t, err)
	purges := func() int {
		n := 0
		for _, c := range fake.Calls() {
			if strings.Contains(c.Query, "resolved_at <=") {
				require.Contains(t, c.Query, "LIMIT 7")
				n++
			}
		}
		return n
	}
	require.NoError(t, queue.Delete(t.Context(), "k"))
	testhelpers.WaitFor(t, 2*time.Second, func() bool { return purges() > 0 }, "the background purge runs")

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	require.NoError(t, hooks.Shutdown(ctx))
	stopped := purges()
	time.Sleep(20 * time.Millisecond)
	require.Equal(t, stopped, purges(), "the shutdown hook stopped the purge")
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
