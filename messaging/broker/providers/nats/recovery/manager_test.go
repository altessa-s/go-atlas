// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"

	natsprovider "github.com/altessa-s/go-atlas/messaging/broker/providers/nats"
)

func newTestProvider(tb testing.TB) *natsprovider.NATS {
	tb.Helper()

	ns := natstest.StartNATSServer(tb)
	nc := natstest.ConnectNATS(tb, ns)
	provider, err := natsprovider.New(nc)
	require.NoError(tb, err)
	return provider
}

// TestNew_WiresComponents pins how Manager options reach its components.
func TestNew_WiresComponents(t *testing.T) {
	t.Parallel()

	provider := newTestProvider(t)
	logger := slog.New(slog.DiscardHandler)

	var successCalled, failureCalled, manualCalled, staleCalled bool
	m, err := New(provider,
		WithLogger(logger),
		WithMaxRecoveryAttempts(7),
		WithRecoveryBackoff(3*time.Second),
		WithStaleRecoveryTimeout(9*time.Minute),
		WithOnRecoverySuccess(func(string, string) { successCalled = true }),
		WithOnRecoveryFailure(func(string, string, error) { failureCalled = true }),
		WithOnManualRecoveryNeeded(func(string, any) { manualCalled = true }),
		WithOnStaleRecoveryCleared(func(string) { staleCalled = true }),
	)
	require.NoError(t, err)
	t.Cleanup(m.Close)

	sup := m.supervisor
	require.Same(t, provider.JetStream(), sup.js)
	require.Same(t, m.registry, sup.registry)
	require.Same(t, logger, sup.logger)
	require.Equal(t, 7, sup.maxAttempts)
	require.Equal(t, 3*time.Second, sup.backoff)

	sup.onRecoverySuccess("s", "c")
	sup.onRecoveryFailure("s", "c", nil)
	sup.onManualNeeded("s", nil)
	sup.onStaleCleared("s")
	require.True(t, successCalled)
	require.True(t, failureCalled)
	require.True(t, manualCalled)
	require.True(t, staleCalled)

	require.Same(t, provider.NATSConn(), m.advisory.nc)
	require.Same(t, m.registry, m.advisory.registry)
	require.Same(t, sup, m.advisory.supervisor)
	require.Same(t, logger, m.advisory.logger)

	require.Same(t, provider.JetStream(), m.healthMon.js)
	require.Same(t, m.registry, m.healthMon.registry)
	require.Same(t, sup, m.healthMon.supervisor)
	require.Same(t, logger, m.healthMon.logger)

	require.Same(t, sup, m.timeoutMon.supervisor)
	require.Same(t, logger, m.timeoutMon.logger)
	require.Equal(t, 9*time.Minute, m.timeoutMon.GetTimeout())
}

// TestNew_Defaults pins the component values when no options are given.
func TestNew_Defaults(t *testing.T) {
	t.Parallel()

	m, err := New(newTestProvider(t))
	require.NoError(t, err)
	t.Cleanup(m.Close)

	require.Equal(t, DefaultMaxRecoveryAttempts, m.supervisor.maxAttempts)
	require.Equal(t, DefaultRecoveryBackoff, m.supervisor.backoff)
	require.Equal(t, DefaultStaleRecoveryTimeout, m.timeoutMon.GetTimeout())
	require.NotNil(t, m.supervisor.logger)
	require.NotNil(t, m.advisory.logger)
	require.NotNil(t, m.healthMon.logger)
	require.NotNil(t, m.timeoutMon.logger)
	require.Nil(t, m.supervisor.onRecoverySuccess)
	require.Nil(t, m.supervisor.onRecoveryFailure)
	require.Nil(t, m.supervisor.onManualNeeded)
	require.Nil(t, m.supervisor.onStaleCleared)
}

// TestComponentConstructors_Defaults pins the defaults of the standalone component constructors.
func TestComponentConstructors_Defaults(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	sup := NewSupervisor(nil, registry)
	t.Cleanup(sup.Close)

	require.Same(t, registry, sup.registry)
	require.NotNil(t, sup.logger)
	require.Equal(t, DefaultMaxRecoveryAttempts, sup.maxAttempts)
	require.Equal(t, DefaultRecoveryBackoff, sup.backoff)

	require.NotNil(t, NewAdvisoryListener(nil, registry, sup).logger)
	require.NotNil(t, NewHealthMonitor(nil, registry, sup).logger)

	tm := NewRecoveryTimeoutMonitor(sup)
	require.NotNil(t, tm.logger)
	require.Equal(t, DefaultStaleRecoveryTimeout, tm.GetTimeout())
}
