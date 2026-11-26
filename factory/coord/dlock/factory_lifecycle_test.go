// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/dlock/providers"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"

	coreruntime "github.com/altessa-s/go-atlas-core/runtime"
	lockconfig "github.com/altessa-s/go-atlas/config/lock"
	dlockfactory "github.com/altessa-s/go-atlas/factory/coord/dlock"
	natsfactory "github.com/altessa-s/go-atlas/factory/coord/dlock/providers/nats"
)

type closeRecordingProvider struct {
	providers.Provider
	closed bool
}

func (p *closeRecordingProvider) Close(context.Context) error {
	p.closed = true
	return nil
}

func TestDLockFactory_ShutdownClosesRegisteredProviders(t *testing.T) {
	t.Parallel()

	t.Run("mongo", func(t *testing.T) {
		t.Parallel()
		var hooks coreruntime.HookGroup
		p := &closeRecordingProvider{}
		cfg := &lockconfig.DistributionLock{Provider: lockconfig.DistributionLockProviderMongo}
		_, err := dlockfactory.New(cfg).UseShutdownHooks(&hooks).
			UseProvider(cfg.Provider, func(context.Context, *lockconfig.DistributionLock, *slog.Logger) (providers.Provider, error) {
				return p, nil
			}).Build(t.Context())
		require.NoError(t, err)
		require.NoError(t, hooks.Shutdown(t.Context()))
		require.True(t, p.closed)
	})

	t.Run("nats", func(t *testing.T) {
		t.Parallel()
		var hooks coreruntime.HookGroup
		nc := natstest.ConnectNATS(t, natstest.StartNATSServer(t))
		cfg := &lockconfig.DistributionLock{
			Provider: lockconfig.DistributionLockProviderNATS,
			NATS:     &lockconfig.DistributionLockNATS{Bucket: "shutdown"},
		}
		dl, err := dlockfactory.New(cfg).UseShutdownHooks(&hooks).
			UseProvider(cfg.Provider, natsfactory.Builder(nc)).Build(t.Context())
		require.NoError(t, err)
		_, err = dl.Lock(t.Context(), "held")
		require.NoError(t, err)
		t.Cleanup(func() { _ = dl.Close(context.WithoutCancel(t.Context())) })

		require.NoError(t, hooks.Shutdown(t.Context()))
		_, err = dl.GetLockInfo(t.Context(), "held")
		require.ErrorIs(t, err, providers.ErrLockNotHeld)
	})
}

func TestDLockFactory_NATSUsesBuilderLogger(t *testing.T) {
	t.Parallel()
	var hooks coreruntime.HookGroup
	var logs bytes.Buffer
	nc := natstest.ConnectNATS(t, natstest.StartNATSServer(t))
	cfg := &lockconfig.DistributionLock{
		Provider: lockconfig.DistributionLockProviderNATS,
		NATS:     &lockconfig.DistributionLockNATS{Bucket: "logger"},
	}
	dl, err := dlockfactory.New(cfg).UseShutdownHooks(&hooks).
		UseLogger(slog.New(slog.NewTextHandler(&logs, nil))).
		UseProvider(cfg.Provider, natsfactory.Builder(nc)).Build(t.Context())
	require.NoError(t, err)
	require.NoError(t, dl.Close(t.Context()))
	require.Contains(t, logs.String(), "closing NATS dlock provider")
}
