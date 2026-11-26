// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/messaging/broker/providers/nats"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
	"github.com/altessa-s/go-atlas/messaging/broker/msg"
	"github.com/altessa-s/go-atlas/messaging/broker/providers/nats/recovery"

	brokerconfig "github.com/altessa-s/go-atlas/config/broker"
	natsconfig "github.com/altessa-s/go-atlas/config/nats"
	brokerfactory "github.com/altessa-s/go-atlas/factory/messaging/broker"
	natsprovider "github.com/altessa-s/go-atlas/messaging/broker/providers/nats"
	schedtask "github.com/altessa-s/go-horai/task"
)

// capturingRegistrar records the scheduler tasks the recovery manager registers.
type capturingRegistrar struct {
	mu         sync.Mutex
	registered []schedtask.Config
}

func (r *capturingRegistrar) Register(_ context.Context, cfg schedtask.Config) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.registered = append(r.registered, cfg)
	return nil
}

func (r *capturingRegistrar) ids() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.registered))
	for _, c := range r.registered {
		out = append(out, c.ID)
	}
	return out
}

// recoveryHandler adapts a function to broker.SubscriberHandler.
type recoveryHandler struct {
	topic  string
	handle func(context.Context, *msg.Message)
}

func (h recoveryHandler) Handle(ctx context.Context, m *msg.Message) { h.handle(ctx, m) }
func (h recoveryHandler) Topic() string                              { return h.topic }

// recoveryEnabledConfig returns a broker config with NATS recovery enabled
// and fast timings so tests don't wait on production-sized backoffs.
func recoveryEnabledConfig() *brokerconfig.Config {
	return &brokerconfig.Config{
		Provider: brokerconfig.ProviderNATS,
		NATS: &natsconfig.Config{
			Recovery: &natsconfig.Recovery{
				Enabled:                      true,
				MaxRecoveryAttempts:          3,
				RecoveryBackoff:              10 * time.Millisecond,
				StaleRecoveryTimeout:         time.Minute,
				HealthCheckSchedule:          "@every 1m",
				StaleRecoveryCleanupSchedule: "@every 1m",
			},
		},
	}
}

func env() brokerfactory.ProviderEnv {
	return brokerfactory.ProviderEnv{Logger: slog.New(slog.DiscardHandler)}
}

func TestBuilder(t *testing.T) {
	t.Parallel()

	t.Run("builds_the_nats_provider", func(t *testing.T) {
		t.Parallel()
		ns := natstest.StartNATSServer(t)
		conn := natstest.ConnectNATS(t, ns)

		provider, err := brokerfactory.New(recoveryEnabledConfig()).
			UseProvider(brokerconfig.ProviderNATS, factory.Builder(conn)).
			CreateProvider()
		require.NoError(t, err)
		require.IsType(t, &natsprovider.NATS{}, provider)
	})

	t.Run("nil_connection_fails", func(t *testing.T) {
		t.Parallel()
		provider, err := factory.Builder(nil)(recoveryEnabledConfig(), env())
		require.ErrorIs(t, err, factory.ErrConnRequired)
		require.Nil(t, provider)
	})
}

// TestNewWithRecovery covers NewWithRecovery, which wires the provider and
// NewRecoveryManager together, against an embedded NATS+JetStream server.
func TestNewWithRecovery(t *testing.T) {
	t.Parallel()

	t.Run("builds_a_working_provider_and_recovery_manager", func(t *testing.T) {
		t.Parallel()
		ns := natstest.StartNATSServer(t)
		conn := natstest.ConnectNATS(t, ns)

		result, err := factory.NewWithRecovery(conn, recoveryEnabledConfig(), env())
		require.NoError(t, err)
		require.NotNil(t, result.Provider)
		require.NotNil(t, result.Recovery)
		t.Cleanup(result.Close)

		// The returned provider must actually work end-to-end: create a
		// stream through the recovery manager, subscribe, publish, and
		// observe the message.
		const streamName, topic = "ORDERS", "orders.created"
		_, err = result.Recovery.CreateStream(t.Context(), jetstream.StreamConfig{Name: streamName, Subjects: []string{"orders.>"}})
		require.NoError(t, err)
		require.True(t, result.Recovery.GetRegistry().HasStream(streamName))

		received := make(chan string, 1)
		handler := recoveryHandler{topic: topic, handle: func(_ context.Context, m *msg.Message) {
			received <- string(m.Data)
			_ = m.Ack()
		}}
		sub := natsprovider.SubscriberWithConsumer(&jetstream.ConsumerConfig{
			Durable: "proc", FilterSubject: topic, AckPolicy: jetstream.AckExplicitPolicy,
		})(result.Provider)
		require.NoError(t, sub.Subscribe(t.Context(), handler))
		t.Cleanup(sub.Unsubscribe)

		require.NoError(t, result.Provider.Publish(t.Context(), *msg.NewMessage(topic, []byte("hi"))))
		select {
		case data := <-received:
			require.Equal(t, "hi", data)
		case <-time.After(5 * time.Second):
			require.FailNow(t, "message was never delivered through the factory-built provider")
		}

		require.NoError(t, result.Recovery.Start())
	})

	t.Run("nil_connection_fails", func(t *testing.T) {
		t.Parallel()
		_, err := factory.NewWithRecovery(nil, recoveryEnabledConfig(), env())
		require.ErrorIs(t, err, factory.ErrConnRequired)
	})
}

// TestNewRecoveryManager covers the config-gating and dependency-validation
// branches of NewRecoveryManager.
func TestNewRecoveryManager(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	conn := natstest.ConnectNATS(t, ns)
	provider, err := natsprovider.New(conn)
	require.NoError(t, err)

	t.Run("nil_config_is_an_error", func(t *testing.T) {
		t.Parallel()
		_, err := factory.NewRecoveryManager(provider, nil, env())
		require.Error(t, err)
	})

	t.Run("nil_nats_config_returns_nothing", func(t *testing.T) {
		t.Parallel()
		mgr, err := factory.NewRecoveryManager(provider, &brokerconfig.Config{}, env())
		require.NoError(t, err)
		require.Nil(t, mgr)
	})

	t.Run("nil_recovery_config_returns_nothing", func(t *testing.T) {
		t.Parallel()
		mgr, err := factory.NewRecoveryManager(provider, &brokerconfig.Config{NATS: &natsconfig.Config{}}, env())
		require.NoError(t, err)
		require.Nil(t, mgr)
	})

	t.Run("disabled_recovery_returns_nothing", func(t *testing.T) {
		t.Parallel()
		cfg := recoveryEnabledConfig()
		cfg.NATS.Recovery.Enabled = false
		mgr, err := factory.NewRecoveryManager(provider, cfg, env())
		require.NoError(t, err)
		require.Nil(t, mgr)
	})

	t.Run("missing_provider_fails", func(t *testing.T) {
		t.Parallel()
		_, err := factory.NewRecoveryManager(nil, recoveryEnabledConfig(), env())
		require.ErrorIs(t, err, factory.ErrProviderRequired)
	})

	t.Run("wires_scheduler_tasks_from_config", func(t *testing.T) {
		t.Parallel()
		reg := &capturingRegistrar{}
		e := env()
		e.Scheduler = reg
		mgr, err := factory.NewRecoveryManager(provider, recoveryEnabledConfig(), e)
		require.NoError(t, err)
		require.NotNil(t, mgr)
		t.Cleanup(mgr.Close)

		ids := reg.ids()
		require.Len(t, ids, 2)
		require.True(t, slices.ContainsFunc(ids, func(id string) bool { return strings.HasPrefix(id, "broker-recovery-health-check-") }), "%v", ids)
		require.True(t, slices.ContainsFunc(ids, func(id string) bool { return strings.HasPrefix(id, "broker-recovery-stale-cleanup-") }), "%v", ids)
	})
}

// TestProviderWithRecovery_Close covers both branches of
// ProviderWithRecovery.Close: a nil Recovery is a no-op, and a real one is
// stopped.
func TestProviderWithRecovery_Close(t *testing.T) {
	t.Parallel()

	t.Run("nil_recovery_is_a_no_op", func(t *testing.T) {
		t.Parallel()
		r := &factory.ProviderWithRecovery{}
		require.NotPanics(t, r.Close)
	})

	t.Run("stops_the_recovery_manager", func(t *testing.T) {
		t.Parallel()
		ns := natstest.StartNATSServer(t)
		conn := natstest.ConnectNATS(t, ns)

		result, err := factory.NewWithRecovery(conn, recoveryEnabledConfig(), env())
		require.NoError(t, err)

		result.Close()
		require.ErrorIs(t, result.Recovery.Start(), recovery.ErrManagerClosed)
		require.NotPanics(t, result.Close, "Close must be idempotent")
	})
}
