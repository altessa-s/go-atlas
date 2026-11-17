// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/reliability/saga"

	sagaconfig "github.com/altessa-s/go-atlas/config/saga"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	sagafactory "github.com/altessa-s/go-atlas/factory/reliability/saga"
	memorystore "github.com/altessa-s/go-atlas/reliability/saga/storages/memory"
)

var errBuild = errors.New("build failed")

type order struct {
	Charged bool
}

func orderDef() *saga.Definition[order] {
	return saga.NewDefinition[order]("place-order").
		Step("charge", func(_ context.Context, o *order) error {
			o.Charged = true
			return nil
		}).
		MustBuild()
}

func TestBuildNilConfig(t *testing.T) {
	t.Parallel()
	_, err := sagafactory.New(nil, orderDef()).Build(t.Context())
	require.ErrorIs(t, err, corefactory.ErrConfigRequired)
}

func TestBuildNilDefinition(t *testing.T) {
	t.Parallel()
	cfg := sagaconfig.Default()
	_, err := sagafactory.New[order](&cfg, nil).Build(t.Context())
	require.ErrorIs(t, err, corefactory.ErrDependencyRequired)
}

func TestBuildMemory(t *testing.T) {
	t.Parallel()
	cfg := sagaconfig.Default()

	orch, err := sagafactory.New(&cfg, orderDef()).Build(t.Context())
	require.NoError(t, err)

	inst, err := orch.Start(t.Context(), "o1", order{})
	require.NoError(t, err)
	require.Equal(t, saga.StatusCompleted, inst.Status)
}

func TestBuildDriverNotRegistered(t *testing.T) {
	t.Parallel()
	for _, storage := range []*sagaconfig.StorageConfig{
		{Type: sagaconfig.StorageTypeNATS, NATS: &sagaconfig.NATSStorageConfig{Bucket: "saga", MaxAge: 720 * time.Hour}},
		{Type: sagaconfig.StorageTypeMongo, Mongo: &sagaconfig.MongoStorageConfig{Collection: "saga_instances"}},
		{Type: sagaconfig.StorageTypeRedis, Redis: &sagaconfig.RedisStorageConfig{KeysPrefix: "saga:"}},
	} {
		t.Run(string(storage.Type), func(t *testing.T) {
			t.Parallel()
			cfg := sagaconfig.Default()
			cfg.Storage = storage

			_, err := sagafactory.New(&cfg, orderDef()).Build(t.Context())
			require.ErrorIs(t, err, corefactory.ErrNotRegistered)
			require.ErrorContains(t, err, "factory/reliability/saga/storages/"+string(storage.Type))
		})
	}
}

func TestUseStorage(t *testing.T) {
	t.Parallel()
	cfg := sagaconfig.Default()
	cfg.Storage = &sagaconfig.StorageConfig{
		Type:  sagaconfig.StorageTypeRedis,
		Redis: &sagaconfig.RedisStorageConfig{KeysPrefix: "saga:"},
	}

	_, err := sagafactory.New(&cfg, orderDef()).UseStorage(sagaconfig.StorageTypeRedis, nil).Build(t.Context())
	require.Error(t, err)

	failing := func(context.Context, *sagaconfig.StorageConfig) (saga.Storage, error) { return nil, errBuild }
	_, err = sagafactory.New(&cfg, orderDef()).UseStorage(sagaconfig.StorageTypeRedis, failing).Build(t.Context())
	require.ErrorIs(t, err, errBuild)

	var called bool
	registered := func(ctx context.Context, got *sagaconfig.StorageConfig) (saga.Storage, error) {
		require.NotNil(t, ctx)
		require.NotSame(t, cfg.Storage, got)
		require.Equal(t, cfg.Storage, got)
		called = true
		return memorystore.New(), nil
	}
	orch, err := sagafactory.New(&cfg, orderDef()).
		UseStorage(sagaconfig.StorageTypeRedis, failing).
		UseStorage(sagaconfig.StorageTypeRedis, registered).
		Build(t.Context())
	require.NoError(t, err)
	require.True(t, called, "the later registration replaces the earlier one")

	inst, err := orch.Start(t.Context(), "o1", order{})
	require.NoError(t, err)
	require.Equal(t, saga.StatusCompleted, inst.Status)
}

func TestBuildInvalidConfig(t *testing.T) {
	t.Parallel()
	cfg := sagaconfig.Default()
	cfg.Storage = &sagaconfig.StorageConfig{Type: "bogus"}

	_, err := sagafactory.New(&cfg, orderDef()).Build(t.Context())
	require.ErrorContains(t, err, "validate saga config")
}

func sqlConfig(dialect string, ensureSchema bool) sagaconfig.Config {
	cfg := sagaconfig.Default()
	cfg.Storage = &sagaconfig.StorageConfig{
		Type: sagaconfig.StorageTypeSQL,
		SQL:  &sagaconfig.SQLStorageConfig{Dialect: dialect, Table: "app.sagas", EnsureSchema: ensureSchema},
	}
	return cfg
}

func TestBuildSQLRequiresDatabase(t *testing.T) {
	t.Parallel()
	cfg := sqlConfig(sagaconfig.SQLDialectPostgres, false)
	_, err := sagafactory.New(&cfg, orderDef()).Build(t.Context())
	require.ErrorIs(t, err, corefactory.ErrDependencyRequired)
}

func TestBuildSQLRejectsUnknownDialect(t *testing.T) {
	t.Parallel()
	db, _ := testhelpers.NewFakeSQL(t, nil)
	cfg := sqlConfig("oracle", false)
	_, err := sagafactory.New(&cfg, orderDef()).UseSQLDB(db).Build(t.Context())
	require.ErrorContains(t, err, "validate saga config")
}

func TestBuildSQL(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		ensureSchema bool
		wantDDL      bool
	}{
		{name: "no schema", ensureSchema: false},
		{name: "ensure schema", ensureSchema: true, wantDDL: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db, fake := testhelpers.NewFakeSQL(t, nil)
			cfg := sqlConfig(sagaconfig.SQLDialectMySQL, tc.ensureSchema)

			_, err := sagafactory.New(&cfg, orderDef()).UseSQLDB(db).Build(t.Context())
			require.NoError(t, err)

			var ddl []string
			for _, c := range fake.Calls() {
				if strings.HasPrefix(c.Query, "CREATE TABLE") {
					ddl = append(ddl, c.Query)
				}
			}
			if !tc.wantDDL {
				require.Empty(t, ddl)
				return
			}
			require.Len(t, ddl, 1)
			require.Contains(t, ddl[0], "`app`.`sagas`")
		})
	}
}

// The caller's context bounds the startup schema creation: a canceled startup
// must not run DDL on a detached context.
func TestBuildSQLHonorsContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	db, fake := testhelpers.NewFakeSQL(t, nil)
	cfg := sqlConfig(sagaconfig.SQLDialectMySQL, true)
	_, err := sagafactory.New(&cfg, orderDef()).UseSQLDB(db).Build(ctx)
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, fake.Calls())
}

func TestBuildDoesNotNormalizeCallerConfig(t *testing.T) {
	t.Parallel()
	cfg := sagaconfig.Default()
	cfg.Storage = &sagaconfig.StorageConfig{Type: sagaconfig.StorageTypeMemory}
	before, storageBefore := cfg, *cfg.Storage
	orch, err := sagafactory.New(&cfg, orderDef()).Build(t.Context())
	require.NoError(t, err)
	require.NotNil(t, orch)
	require.Equal(t, before, cfg)
	require.Equal(t, storageBefore, *cfg.Storage)
}
