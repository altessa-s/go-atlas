// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/leadelect/providers"
	"github.com/altessa-s/go-atlas/factory/coord/leadelect"
	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"

	lockconfig "github.com/altessa-s/go-atlas/config/lock"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

var errBuild = errors.New("build failed")

// fakeProvider stands in for a registered provider; Build calls none of its
// methods.
type fakeProvider struct{ providers.Provider }

func TestBuild_RejectsMissingNodeID(t *testing.T) {
	t.Parallel()
	cfg := &lockconfig.LeaderElector{Provider: lockconfig.LeaderElectorProviderNATS, Ttl: time.Second}
	_, err := factory.New(cfg).WithKey("election").
		UseProvider(cfg.Provider, func(context.Context, *lockconfig.LeaderElector, *slog.Logger, metrics.Meter) (providers.Provider, error) {
			return fakeProvider{}, nil
		}).Build(t.Context())
	require.ErrorIs(t, err, corefactory.ErrConfigRequired)
}

func TestBuild_RequiresConfigAndRegisteredProvider(t *testing.T) {
	t.Parallel()

	_, err := factory.New(nil).Build(t.Context())
	require.ErrorIs(t, err, corefactory.ErrConfigRequired)

	_, err = factory.New(&lockconfig.LeaderElector{Ttl: time.Second}).WithNodeId("node").Build(t.Context())
	require.ErrorIs(t, err, corefactory.ErrNotRegistered, "an empty provider is nats")

	_, err = factory.New(&lockconfig.LeaderElector{Provider: "etcd", Ttl: time.Second}).WithNodeId("node").Build(t.Context())
	require.ErrorIs(t, err, corefactory.ErrUnsupported)
}

func TestUseProvider(t *testing.T) {
	t.Parallel()
	cfg := &lockconfig.LeaderElector{Provider: lockconfig.LeaderElectorProviderNATS, Ttl: 5 * time.Second}

	_, err := factory.New(cfg).UseProvider(lockconfig.LeaderElectorProviderNATS, nil).Build(t.Context())
	require.Error(t, err)

	failing := func(context.Context, *lockconfig.LeaderElector, *slog.Logger, metrics.Meter) (providers.Provider, error) {
		return nil, errBuild
	}
	_, err = factory.New(cfg).WithNodeId("node").UseProvider(lockconfig.LeaderElectorProviderNATS, failing).Build(t.Context())
	require.ErrorIs(t, err, errBuild)

	logger := slog.New(slog.DiscardHandler)
	tc := metricstest.NewTestCollector()
	var called bool
	registered := func(ctx context.Context, got *lockconfig.LeaderElector, l *slog.Logger, m metrics.Meter) (providers.Provider, error) {
		called = true
		require.NotNil(t, ctx)
		require.Same(t, cfg, got)
		require.Same(t, logger, l)
		require.Equal(t, metrics.Meter(tc), m)
		return fakeProvider{}, nil
	}
	leader, err := factory.New(cfg).
		WithNodeId("node").
		UseLogger(logger).
		UseMeter(tc).
		UseProvider(lockconfig.LeaderElectorProviderNATS, failing).
		UseProvider(lockconfig.LeaderElectorProviderNATS, registered).
		Build(t.Context())
	require.NoError(t, err)
	require.NotNil(t, leader)
	require.True(t, called, "the registered provider function must build the provider")
}
