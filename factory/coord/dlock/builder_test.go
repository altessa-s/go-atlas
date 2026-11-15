// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/dlock/providers"
	"github.com/altessa-s/go-atlas/factory/coord/dlock"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	lockconfig "github.com/altessa-s/go-atlas/config/lock"
	noopprovider "github.com/altessa-s/go-atlas/coord/dlock/providers/noop"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

var errBuild = errors.New("build failed")

func TestBuild_ProviderErrors(t *testing.T) {
	t.Parallel()

	_, err := factory.New(nil).Build(t.Context())
	require.Error(t, err, "nil config")

	for _, typ := range []lockconfig.DistributionLockProvider{lockconfig.DistributionLockProviderNATS, lockconfig.DistributionLockProviderMongo} {
		_, err := factory.New(&lockconfig.DistributionLock{Provider: typ}).Build(t.Context())
		require.ErrorIs(t, err, corefactory.ErrNotRegistered, typ)
	}

	_, err = factory.New(&lockconfig.DistributionLock{Provider: "etcd"}).Build(t.Context())
	require.ErrorIs(t, err, corefactory.ErrUnsupported)
}

func TestUseProvider(t *testing.T) {
	t.Parallel()
	cfg := &lockconfig.DistributionLock{Provider: lockconfig.DistributionLockProviderNATS}

	_, err := factory.New(cfg).UseProvider(lockconfig.DistributionLockProviderNATS, nil).Build(t.Context())
	require.Error(t, err)

	failing := func(context.Context, *lockconfig.DistributionLock, *slog.Logger) (providers.Provider, error) {
		return nil, errBuild
	}
	_, err = factory.New(cfg).UseProvider(lockconfig.DistributionLockProviderNATS, failing).Build(t.Context())
	require.ErrorIs(t, err, errBuild)

	logger := slog.New(slog.DiscardHandler)
	var called bool
	registered := func(ctx context.Context, got *lockconfig.DistributionLock, l *slog.Logger) (providers.Provider, error) {
		called = true
		require.NotNil(t, ctx)
		require.Same(t, cfg, got)
		require.Same(t, logger, l)
		return noopprovider.New(), nil
	}
	dl, err := factory.New(cfg).
		UseLogger(logger).
		UseProvider(lockconfig.DistributionLockProviderNATS, failing).
		UseProvider(lockconfig.DistributionLockProviderNATS, registered).
		Build(t.Context())
	require.NoError(t, err)
	require.NotNil(t, dl)
	require.True(t, called, "the registered provider function must build the provider")
}

func TestBuild_SQL(t *testing.T) {
	t.Parallel()
	sqlCfg := func(ensure bool) *lockconfig.DistributionLock {
		return &lockconfig.DistributionLock{Provider: lockconfig.DistributionLockProviderSQL,
			SQL: &lockconfig.DistributionLockSQL{Dialect: lockconfig.SQLDialectPostgres, Table: "app.locks", EnsureSchema: ensure}}
	}

	t.Run("requires_database", func(t *testing.T) {
		t.Parallel()
		_, err := factory.New(sqlCfg(false)).Build(t.Context())
		require.ErrorIs(t, err, corefactory.ErrDependencyRequired)
	})
	t.Run("requires_section", func(t *testing.T) {
		t.Parallel()
		db, _ := testhelpers.NewFakeSQL(t, nil)
		_, err := factory.New(&lockconfig.DistributionLock{Provider: lockconfig.DistributionLockProviderSQL}).
			UseSQLDB(db).Build(t.Context())
		require.Error(t, err)
	})
	for _, ensure := range []bool{false, true} {
		t.Run("ensure_schema_"+strconv.FormatBool(ensure), func(t *testing.T) {
			t.Parallel()
			db, fake := testhelpers.NewFakeSQL(t, nil)
			dl, err := factory.New(sqlCfg(ensure)).UseSQLDB(db).Build(t.Context())
			require.NoError(t, err)
			require.NotNil(t, dl)
			var ddl int
			for _, c := range fake.Calls() {
				if strings.Contains(c.Query, `CREATE TABLE IF NOT EXISTS "app"."locks"`) {
					ddl++
				}
			}
			require.Equal(t, map[bool]int{false: 0, true: 1}[ensure], ddl)
		})
	}
}
