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
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/dlock/providers"
	"github.com/altessa-s/go-atlas/factory/coord/dlock"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	coreruntime "github.com/altessa-s/go-atlas-core/runtime"
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

// TestBuild_SQLOptionsFromConfig pins that the lease settings of the sqldb
// block reach the provider.
func TestBuild_SQLOptionsFromConfig(t *testing.T) {
	t.Parallel()
	build := func(c lockconfig.DistributionLockSQL) error {
		db, _ := testhelpers.NewFakeSQL(t, nil)
		_, err := factory.New(&lockconfig.DistributionLock{Provider: lockconfig.DistributionLockProviderSQL, SQL: &c}).
			UseSQLDB(db).Build(t.Context())
		return err
	}
	base := func() lockconfig.DistributionLockSQL {
		return lockconfig.DistributionLockSQL{Dialect: lockconfig.SQLDialectPostgres, Table: "dlocks", TTL: 10 * time.Second}
	}

	hard := base()
	hard.WithoutRenewal, hard.ExpiredCleanup, hard.ExpiredCleanupGrace = true, true, time.Minute
	require.NoError(t, build(hard), "a hard lease with cleanup builds without I/O")

	tooShort := base()
	tooShort.TTL = 500 * time.Microsecond
	require.Error(t, build(tooShort), "the TTL reaches the provider")

	badRatio := base()
	badRatio.TTL, badRatio.RenewRatio = 2*time.Millisecond, 0.1
	require.Error(t, build(badRatio), "the renew ratio reaches the provider")
	badRatio.WithoutRenewal = true
	require.NoError(t, build(badRatio), "a hard lease ignores the renew ratio")

	negative := base()
	negative.ExpiredCleanup, negative.ExpiredCleanupGrace = true, -time.Second
	require.Error(t, build(negative), "the cleanup grace reaches the provider")
}

func TestBuild_SQLPurgeShutdownAndBatch(t *testing.T) {
	t.Parallel()
	db, fake := testhelpers.NewFakeSQL(t, nil)
	var hooks coreruntime.HookGroup
	cfg := &lockconfig.DistributionLock{Provider: lockconfig.DistributionLockProviderSQL,
		SQL: &lockconfig.DistributionLockSQL{Dialect: "postgres", Table: "locks", ExpiredCleanup: true,
			PurgeInterval: time.Millisecond, PurgeBatchSize: 7}}
	dl, err := factory.New(cfg).UseSQLDB(db).UseShutdownHooks(&hooks).Build(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, hooks.Shutdown(context.WithoutCancel(t.Context()))) })
	require.Empty(t, fake.Calls(), "Build starts no goroutine")
	_, _ = dl.Lock(t.Context(), "key") // The fake refuses acquisition; this is still a write-path call.
	testhelpers.WaitFor(t, time.Second, func() bool {
		for _, call := range fake.Calls() {
			if strings.HasPrefix(call.Query, "DELETE") {
				return true
			}
		}
		return false
	}, "configured purge interval reaches the provider")
	require.NoError(t, hooks.Shutdown(t.Context()))
	before := len(fake.Calls())
	_, err = dl.Lock(t.Context(), "after-shutdown")
	require.Error(t, err, "shutdown closes the lease engine")
	time.Sleep(10 * time.Millisecond)
	require.Len(t, fake.Calls(), before, "purging cannot restart")
	for _, call := range fake.Calls() {
		if strings.HasPrefix(call.Query, "DELETE") {
			require.Equal(t, []any{int64(0), int64(7)}, call.Args, "configured batch size is bound")
		}
	}
}

func TestBuildCustomBackend(t *testing.T) {
	t.Parallel()
	cfg := &lockconfig.DistributionLock{Provider: "custom"}

	build := func(context.Context, *lockconfig.DistributionLock, *slog.Logger) (providers.Provider, error) {
		return noopprovider.New(), nil
	}
	b := factory.New(cfg).UseProvider("custom", build)
	cfg.SetRegisteredTypes(b.RegisteredProviderTypes())
	require.NoError(t, cfg.Validate())
	got, err := b.Build(t.Context())
	require.NoError(t, err)
	require.NotNil(t, got)
	_, err = factory.New(cfg).Build(t.Context())
	require.ErrorIs(t, err, corefactory.ErrUnsupported)
}
