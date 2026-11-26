// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery_test

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
	"github.com/altessa-s/go-atlas/messaging/broker"
	"github.com/altessa-s/go-atlas/messaging/broker/msg"
	"github.com/altessa-s/go-atlas/messaging/broker/providers/nats/recovery"

	natsprovider "github.com/altessa-s/go-atlas/messaging/broker/providers/nats"
)

type nopHandler struct{ topic string }

func (nopHandler) Handle(context.Context, *msg.Message) {}
func (h nopHandler) Topic() string                      { return h.topic }

type ephemeralFixture struct {
	manager *recovery.Manager
	js      jetstream.JetStream
	stream  jetstream.Stream
	created atomic.Int32
	factory natsprovider.SubscriberFactory
}

func newEphemeralFixture(t *testing.T) *ephemeralFixture {
	t.Helper()
	ns := natstest.StartNATSServer(t)
	conn, js := natstest.ConnectJetStream(t, ns)
	provider, err := natsprovider.New(conn)
	require.NoError(t, err)
	manager, err := recovery.New(provider,
		recovery.WithLogger(slog.New(slog.DiscardHandler)),
		recovery.WithRecoveryBackoff(time.Millisecond),
	)
	require.NoError(t, err)
	t.Cleanup(manager.Close)

	f := &ephemeralFixture{manager: manager, js: js}
	f.stream, err = manager.CreateStream(t.Context(), jetstream.StreamConfig{Name: "ORDERS", Subjects: []string{"orders.>"}})
	require.NoError(t, err)
	f.factory = func(p *natsprovider.NATS) broker.Subscriber {
		f.created.Add(1)
		return natsprovider.SubscriberWithEphemeralConsumer()(p)
	}
	return f
}

func (f *ephemeralFixture) serverConsumers(t *testing.T) []string {
	t.Helper()
	var names []string
	for info := range f.stream.ListConsumers(t.Context()).Info() {
		names = append(names, info.Name)
	}
	return names
}

func (f *ephemeralFixture) registered() []string {
	return slices.Sorted(f.manager.GetRegistry().ConsumerNames("ORDERS"))
}

// TestManager_EphemeralConsumerIsKeyedByServerName verifies that the registry
// is keyed by the name the server assigned to an ephemeral consumer, so the
// health check finds it and does not resubscribe on every tick.
func TestManager_EphemeralConsumerIsKeyedByServerName(t *testing.T) {
	t.Parallel()

	f := newEphemeralFixture(t)
	_, err := f.manager.Subscribe(t.Context(), "ORDERS", "label", nopHandler{topic: "orders.created"}, f.factory)
	require.NoError(t, err)

	require.Equal(t, f.serverConsumers(t), f.registered(), "the registry must hold the server-assigned name")

	for range 2 {
		require.NoError(t, f.manager.RunHealthCheckCycle(t.Context()))
	}
	require.Never(t, func() bool { return f.created.Load() > 1 }, 500*time.Millisecond, 10*time.Millisecond,
		"a healthy ephemeral consumer must not be resubscribed")
}

// TestManager_EphemeralReplacementIsRekeyed verifies that every recovery of
// an ephemeral consumer retires the old name and registers the new one.
func TestManager_EphemeralReplacementIsRekeyed(t *testing.T) {
	t.Parallel()

	f := newEphemeralFixture(t)
	_, err := f.manager.Subscribe(t.Context(), "ORDERS", "label", nopHandler{topic: "orders.created"}, f.factory)
	require.NoError(t, err)

	for round := range 2 {
		old := f.serverConsumers(t)
		require.Len(t, old, 1, "round %d", round)
		require.NoError(t, f.js.DeleteConsumer(t.Context(), "ORDERS", old[0]))
		require.NoError(t, f.manager.RunHealthCheckCycle(t.Context()))

		require.Eventually(t, func() bool {
			names := f.serverConsumers(t)
			return len(names) == 1 && names[0] != old[0] && slices.Equal(names, f.registered())
		}, 5*time.Second, 10*time.Millisecond, "round %d: the replacement must be registered under its own name", round)
		require.Equal(t, 1, f.manager.GetRegistry().ConsumerCount("ORDERS"))
		require.False(t, f.manager.GetRegistry().HasConsumer("ORDERS", old[0]))
	}
}

// TestSupervisor_RecoverConsumerSingleFlight verifies that overlapping
// RecoverConsumer calls for one consumer run a single recovery.
func TestSupervisor_RecoverConsumerSingleFlight(t *testing.T) {
	t.Parallel()

	registry := recovery.NewRegistry()
	registry.RegisterStream("s", jetstream.StreamConfig{Name: "s"}, recovery.RecoveryStrategyAuto)
	var calls atomic.Int32
	entered := make(chan struct{})
	release := make(chan struct{})
	registry.RegisterResubscribeHandler("s", "c", func(context.Context) error {
		if calls.Add(1) == 1 {
			close(entered)
		}
		<-release
		return nil
	})
	sup := recovery.NewSupervisor(nil, registry, recovery.WithSupervisorLogger(slog.New(slog.DiscardHandler)))

	sup.RecoverConsumer("s", "c", nil)
	<-entered
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() { sup.RecoverConsumer("s", "c", nil) })
	}
	wg.Wait()
	close(release)
	sup.Close()

	require.Equal(t, int32(1), calls.Load())
}

// TestSupervisor_CloseRacesRecover exercises Close against concurrent
// RecoverConsumer calls (run with -race).
func TestSupervisor_CloseRacesRecover(t *testing.T) {
	t.Parallel()

	for range 50 {
		registry := recovery.NewRegistry()
		registry.RegisterStream("s", jetstream.StreamConfig{Name: "s"}, recovery.RecoveryStrategyAuto)
		registry.RegisterResubscribeHandler("s", "c", func(context.Context) error { return nil })
		sup := recovery.NewSupervisor(nil, registry, recovery.WithSupervisorLogger(slog.New(slog.DiscardHandler)))

		var wg sync.WaitGroup
		for range 4 {
			wg.Go(func() { sup.RecoverConsumer("s", "c", nil) })
		}
		sup.Close()
		wg.Wait()
		sup.Close()
	}
}
