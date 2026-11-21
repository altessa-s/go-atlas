// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-aip/requestid"
	"github.com/altessa-s/go-aip/requestid/requestidtest"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/redistest"

	coreruntime "github.com/altessa-s/go-atlas-core/runtime"
	cfg "github.com/altessa-s/go-atlas/config/requestidstore"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	factory "github.com/altessa-s/go-atlas/factory/reliability/requestidstore"
	redisfactory "github.com/altessa-s/go-atlas/factory/reliability/requestidstore/storages/redis"
)

func TestBuild(t *testing.T) {
	t.Parallel()
	_, err := factory.New(nil).Build(t.Context())
	require.ErrorIs(t, err, corefactory.ErrConfigRequired)
	c := &cfg.Config{Type: cfg.StorageTypeRedis, Redis: &storageconfig.RedisConfig{KeysPrefix: "orders:"}}
	b := factory.New(c)
	_, err = b.Build(t.Context())
	require.ErrorIs(t, err, corefactory.ErrNotRegistered)
	client, server := redistest.RedisClient(t)
	s, err := b.UseStorage(c.Type, redisfactory.Builder(client)).Build(t.Context())
	require.NoError(t, err)
	key := requestidtest.Key(t.Name())
	_, ok, err := s.Reserve(t.Context(), key, requestid.Entry{State: requestid.StatePending}, requestidtest.Base)
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, server.Exists("orders:"+key))
}

func TestSQLDependency(t *testing.T) {
	t.Parallel()
	c := &cfg.Config{Type: cfg.StorageTypeSQL, SQL: &cfg.SQLConfig{Table: "request_ids", Dialect: "postgres", PurgeBatchSize: 1}}
	_, err := factory.New(c).Build(t.Context())
	require.ErrorIs(t, err, corefactory.ErrDependencyRequired)
}

func TestSQLSchema(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, nil)
	c := &cfg.Config{Type: cfg.StorageTypeSQL, SQL: &cfg.SQLConfig{
		Dialect: "mysql", Table: "orders_request_ids", EnsureSchema: true, PurgeBatchSize: 1,
	}}
	var hooks coreruntime.HookGroup
	store, err := factory.New(c).UseSQLDB(db).UseShutdownHooks(&hooks).Build(t.Context())
	require.NoError(t, err)
	require.NotNil(t, store)
	require.Len(t, fake.Calls(), 1)
	require.Contains(t, fake.Calls()[0].Query, "orders_request_ids")
	require.NoError(t, hooks.Shutdown(t.Context()))
}
