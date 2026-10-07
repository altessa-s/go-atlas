// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/saga"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	sagaconfig "github.com/altessa-s/go-atlas/config/saga"
	sagafactory "github.com/altessa-s/go-atlas/data/saga/factory"
)

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
	_, err := sagafactory.New(nil, orderDef()).Build()
	require.ErrorContains(t, err, "configuration is required")
}

func TestBuildNilDefinition(t *testing.T) {
	t.Parallel()
	cfg := sagaconfig.Default()
	_, err := sagafactory.New[order](&cfg, nil).Build()
	require.ErrorContains(t, err, "definition is required")
}

func TestBuildMemory(t *testing.T) {
	t.Parallel()
	cfg := sagaconfig.Default()

	orch, err := sagafactory.New(&cfg, orderDef()).Build()
	require.NoError(t, err)

	inst, err := orch.Start(t.Context(), "o1", order{})
	require.NoError(t, err)
	require.Equal(t, saga.StatusCompleted, inst.Status)
}

func TestBuildNatsRequiresJetStream(t *testing.T) {
	t.Parallel()
	cfg := sagaconfig.Default()
	cfg.Storage = &sagaconfig.StorageConfig{
		Type: sagaconfig.StorageTypeNATS,
		NATS: &sagaconfig.NATSStorageConfig{Bucket: "saga", MaxAge: 720 * time.Hour},
	}

	_, err := sagafactory.New(&cfg, orderDef()).Build()
	require.ErrorContains(t, err, "NATS JetStream is required")
}

func TestBuildMongoRequiresDatabase(t *testing.T) {
	t.Parallel()
	cfg := sagaconfig.Default()
	cfg.Storage = &sagaconfig.StorageConfig{
		Type:  sagaconfig.StorageTypeMongo,
		Mongo: &sagaconfig.MongoStorageConfig{Collection: "saga_instances"},
	}

	_, err := sagafactory.New(&cfg, orderDef()).Build()
	require.ErrorContains(t, err, "MongoDB database is required")
}

func TestBuildRedisRequiresClient(t *testing.T) {
	t.Parallel()
	cfg := sagaconfig.Default()
	cfg.Storage = &sagaconfig.StorageConfig{
		Type:  sagaconfig.StorageTypeRedis,
		Redis: &sagaconfig.RedisStorageConfig{KeysPrefix: "saga:"},
	}

	_, err := sagafactory.New(&cfg, orderDef()).Build()
	require.ErrorContains(t, err, "Redis client is required")
}

func TestBuildInvalidConfig(t *testing.T) {
	t.Parallel()
	cfg := sagaconfig.Default()
	cfg.Storage = &sagaconfig.StorageConfig{Type: "bogus"}

	_, err := sagafactory.New(&cfg, orderDef()).Build()
	require.ErrorContains(t, err, "validate saga config")
}

func TestBuildRedis(t *testing.T) {
	t.Parallel()
	client, _ := testhelpers.RedisClient(t)

	cfg := sagaconfig.Default()
	cfg.Storage = &sagaconfig.StorageConfig{
		Type:  sagaconfig.StorageTypeRedis,
		Redis: &sagaconfig.RedisStorageConfig{KeysPrefix: "saga:"},
	}

	orch, err := sagafactory.New(&cfg, orderDef()).
		UseRedisClient(client).
		Build()
	require.NoError(t, err)

	inst, err := orch.Start(t.Context(), "o1", order{})
	require.NoError(t, err)
	require.Equal(t, saga.StatusCompleted, inst.Status)
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
	_, err := sagafactory.New(&cfg, orderDef()).Build()
	require.ErrorContains(t, err, "SQL database is required")
}

func TestBuildSQLRejectsUnknownDialect(t *testing.T) {
	t.Parallel()
	db, _ := testhelpers.NewFakeSQL(t, nil)
	cfg := sqlConfig("oracle", false)
	_, err := sagafactory.New(&cfg, orderDef()).UseSQLDB(db).Build()
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

			_, err := sagafactory.New(&cfg, orderDef()).UseSQLDB(db).Build()
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
