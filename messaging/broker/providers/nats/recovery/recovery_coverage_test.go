// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
	"github.com/altessa-s/go-atlas/messaging/broker/msg"
	"github.com/altessa-s/go-atlas/messaging/broker/providers/nats/recovery"

	natsprovider "github.com/altessa-s/go-atlas/messaging/broker/providers/nats"
)

// funcHandler adapts a function to broker.SubscriberHandler.
type funcHandler struct {
	topic  string
	handle func(context.Context, *msg.Message)
}

func (h funcHandler) Handle(ctx context.Context, m *msg.Message) {
	if h.handle != nil {
		h.handle(ctx, m)
	}
}
func (h funcHandler) Topic() string { return h.topic }

// newRecoveryTestProvider starts an embedded NATS+JetStream server and
// returns a connected provider suitable for recovery.New.
func newRecoveryTestProvider(tb testing.TB) *natsprovider.NATS {
	tb.Helper()

	ns := natstest.StartNATSServer(tb)
	nc := natstest.ConnectNATS(tb, ns)
	provider, err := natsprovider.New(nc)
	require.NoError(tb, err)
	return provider
}

// --- Manager.RegisterStream / UnregisterStream, newStreamOptions ---

func TestManager_RegisterStream(t *testing.T) {
	t.Parallel()

	t.Run("empty_name_is_rejected", func(t *testing.T) {
		t.Parallel()
		m, err := recovery.New(newRecoveryTestProvider(t))
		require.NoError(t, err)
		t.Cleanup(m.Close)

		require.ErrorIs(t, m.RegisterStream(jetstream.StreamConfig{}), recovery.ErrInvalidStreamConfig)
	})

	t.Run("default_strategy_is_auto", func(t *testing.T) {
		t.Parallel()
		m, err := recovery.New(newRecoveryTestProvider(t))
		require.NoError(t, err)
		t.Cleanup(m.Close)

		require.NoError(t, m.RegisterStream(jetstream.StreamConfig{Name: "S1"}))
		require.True(t, m.GetRegistry().GetRecoveryStrategy("S1").IsAuto())
	})

	t.Run("explicit_strategy_overrides_default", func(t *testing.T) {
		t.Parallel()
		m, err := recovery.New(newRecoveryTestProvider(t))
		require.NoError(t, err)
		t.Cleanup(m.Close)

		require.NoError(t, m.RegisterStream(jetstream.StreamConfig{Name: "S2"},
			recovery.WithRecoveryStrategy(recovery.RecoveryStrategyManual)))
		require.True(t, m.GetRegistry().GetRecoveryStrategy("S2").IsManual())
	})

	t.Run("rejected_after_close", func(t *testing.T) {
		t.Parallel()
		m, err := recovery.New(newRecoveryTestProvider(t))
		require.NoError(t, err)
		m.Close()

		require.ErrorIs(t, m.RegisterStream(jetstream.StreamConfig{Name: "S3"}), recovery.ErrManagerClosed)
	})
}

func TestManager_UnregisterStream(t *testing.T) {
	t.Parallel()

	m, err := recovery.New(newRecoveryTestProvider(t))
	require.NoError(t, err)
	t.Cleanup(m.Close)

	require.NoError(t, m.RegisterStream(jetstream.StreamConfig{Name: "S1"}))
	require.True(t, m.GetRegistry().HasStream("S1"))

	m.UnregisterStream("S1")
	require.False(t, m.GetRegistry().HasStream("S1"))
}

// --- Manager.CreateStream ---

func TestManager_CreateStream(t *testing.T) {
	t.Parallel()

	t.Run("creates_and_registers", func(t *testing.T) {
		t.Parallel()
		provider := newRecoveryTestProvider(t)
		m, err := recovery.New(provider)
		require.NoError(t, err)
		t.Cleanup(m.Close)

		stream, err := m.CreateStream(t.Context(), jetstream.StreamConfig{Name: "ORDERS", Subjects: []string{"orders.>"}})
		require.NoError(t, err)
		require.NotNil(t, stream)
		require.True(t, m.GetRegistry().HasStream("ORDERS"))

		got, err := provider.JetStream().Stream(t.Context(), "ORDERS")
		require.NoError(t, err)
		require.Equal(t, "ORDERS", got.CachedInfo().Config.Name)
	})

	t.Run("failure_unregisters", func(t *testing.T) {
		t.Parallel()
		provider := newRecoveryTestProvider(t)
		m, err := recovery.New(provider)
		require.NoError(t, err)
		t.Cleanup(m.Close)

		_, err = provider.JetStream().CreateStream(t.Context(), jetstream.StreamConfig{Name: "EXISTING", Subjects: []string{"dup.topic"}})
		require.NoError(t, err)

		// A second stream claiming the exact same subject is rejected by the server.
		_, err = m.CreateStream(t.Context(), jetstream.StreamConfig{Name: "OVERLAP", Subjects: []string{"dup.topic"}})
		require.Error(t, err)
		require.False(t, m.GetRegistry().HasStream("OVERLAP"), "a failed create must not leave the stream registered")
	})

	t.Run("rejected_after_close", func(t *testing.T) {
		t.Parallel()
		m, err := recovery.New(newRecoveryTestProvider(t))
		require.NoError(t, err)
		m.Close()

		_, err = m.CreateStream(t.Context(), jetstream.StreamConfig{Name: "S1"})
		require.ErrorIs(t, err, recovery.ErrManagerClosed)
	})
}

// --- Manager.Start / Close ---

func TestManager_Start(t *testing.T) {
	t.Parallel()

	t.Run("starts_advisory_listener", func(t *testing.T) {
		t.Parallel()
		m, err := recovery.New(newRecoveryTestProvider(t))
		require.NoError(t, err)
		t.Cleanup(m.Close)

		require.NoError(t, m.Start())
	})

	t.Run("rejected_after_close", func(t *testing.T) {
		t.Parallel()
		m, err := recovery.New(newRecoveryTestProvider(t))
		require.NoError(t, err)
		m.Close()

		require.ErrorIs(t, m.Start(), recovery.ErrManagerClosed)
	})
}

// --- AdvisoryListener: real deletions drive recovery ---

// TestAdvisoryListener_RecoversStreamOnRealDeletion deletes a registered
// stream directly on the server and checks that the server's own advisory
// event reaches AdvisoryListener.handleStreamDeleted and triggers recovery.
func TestAdvisoryListener_RecoversStreamOnRealDeletion(t *testing.T) {
	t.Parallel()

	provider := newRecoveryTestProvider(t)
	m, err := recovery.New(provider, recovery.WithRecoveryBackoff(10*time.Millisecond))
	require.NoError(t, err)
	t.Cleanup(m.Close)
	require.NoError(t, m.Start())

	const name = "ORDERS"
	_, err = m.CreateStream(t.Context(), jetstream.StreamConfig{Name: name, Subjects: []string{"orders.>"}})
	require.NoError(t, err)

	require.NoError(t, provider.JetStream().DeleteStream(t.Context(), name))

	testhelpers.WaitFor(t, 5*time.Second, func() bool {
		_, err := provider.JetStream().Stream(t.Context(), name)
		return err == nil
	}, "deleted stream must be recreated by the advisory-driven recovery")
}

// TestAdvisoryListener_HandleStreamDeleted_SubjectFallback simulates an
// advisory payload that omits the "stream" field, forcing
// extractStreamNameFromSubject to supply the stream name instead.
func TestAdvisoryListener_HandleStreamDeleted_SubjectFallback(t *testing.T) {
	t.Parallel()

	provider := newRecoveryTestProvider(t)
	m, err := recovery.New(provider, recovery.WithRecoveryBackoff(10*time.Millisecond))
	require.NoError(t, err)
	t.Cleanup(m.Close)
	require.NoError(t, m.Start())

	const name = "FALLBACK"
	require.NoError(t, m.RegisterStream(jetstream.StreamConfig{Name: name, Subjects: []string{"fallback.>"}}))

	payload, err := json.Marshal(map[string]string{
		"type": "io.nats.jetstream.advisory.v1.stream_deleted",
		"id":   "test-event",
	})
	require.NoError(t, err)

	nc := provider.NATSConn()
	require.NoError(t, nc.Publish(fmt.Sprintf("$JS.EVENT.ADVISORY.STREAM.DELETED.%s", name), payload))
	require.NoError(t, nc.Flush())

	testhelpers.WaitFor(t, 5*time.Second, func() bool {
		_, err := provider.JetStream().Stream(t.Context(), name)
		return err == nil
	}, "a subject-only stream name must still trigger recovery")
}

// TestAdvisoryListener_IgnoresUnregisteredStream checks that deleting a
// stream the manager never registered leaves it deleted: the listener's
// "ignoring deletion for unregistered stream" branch must not recreate it.
func TestAdvisoryListener_IgnoresUnregisteredStream(t *testing.T) {
	t.Parallel()

	provider := newRecoveryTestProvider(t)
	m, err := recovery.New(provider, recovery.WithRecoveryBackoff(10*time.Millisecond))
	require.NoError(t, err)
	t.Cleanup(m.Close)
	require.NoError(t, m.Start())

	const name = "UNREGISTERED"
	_, err = provider.JetStream().CreateStream(t.Context(), jetstream.StreamConfig{Name: name, Subjects: []string{"unreg.>"}})
	require.NoError(t, err)
	require.NoError(t, provider.JetStream().DeleteStream(t.Context(), name))

	// Barrier: one subscription handles deletion advisories in order, so once
	// a registered stream deleted afterwards is recreated, the advisory for
	// the unregistered one has been handled.
	const sentinel = "UNREGISTERED_SENTINEL"
	_, err = m.CreateStream(t.Context(), jetstream.StreamConfig{Name: sentinel, Subjects: []string{"unreg-sentinel.>"}})
	require.NoError(t, err)
	require.NoError(t, provider.JetStream().DeleteStream(t.Context(), sentinel))
	testhelpers.WaitFor(t, 5*time.Second, func() bool {
		_, err := provider.JetStream().Stream(t.Context(), sentinel)
		return err == nil
	}, "the registered sentinel stream is recreated")

	_, err = provider.JetStream().Stream(t.Context(), name)
	require.ErrorIs(t, err, jetstream.ErrStreamNotFound, "a stream never registered for recovery must stay deleted")
}

// TestAdvisoryListener_IgnoresMalformedPayload checks that a malformed
// advisory payload is logged and dropped rather than crashing the listener,
// which must keep handling subsequent, well-formed events.
func TestAdvisoryListener_IgnoresMalformedPayload(t *testing.T) {
	t.Parallel()

	provider := newRecoveryTestProvider(t)
	m, err := recovery.New(provider, recovery.WithRecoveryBackoff(10*time.Millisecond))
	require.NoError(t, err)
	t.Cleanup(m.Close)
	require.NoError(t, m.Start())

	const name = "MALFORMED"
	_, err = m.CreateStream(t.Context(), jetstream.StreamConfig{Name: name, Subjects: []string{"malformed.>"}})
	require.NoError(t, err)

	nc := provider.NATSConn()
	require.NoError(t, nc.Publish("$JS.EVENT.ADVISORY.STREAM.DELETED."+name, []byte("not json")))
	require.NoError(t, nc.Flush())

	require.NoError(t, provider.JetStream().DeleteStream(t.Context(), name))
	testhelpers.WaitFor(t, 5*time.Second, func() bool {
		_, err := provider.JetStream().Stream(t.Context(), name)
		return err == nil
	}, "listener must still recover streams after a malformed advisory payload")
}

// --- Manager.Subscribe / ManagedSubscription ---

func TestManager_Subscribe_Validations(t *testing.T) {
	t.Parallel()

	provider := newRecoveryTestProvider(t)
	m, err := recovery.New(provider)
	require.NoError(t, err)
	t.Cleanup(m.Close)

	factory := natsprovider.SubscriberWithConsumer(&jetstream.ConsumerConfig{Durable: "c1"})
	handler := funcHandler{topic: "x.>"}

	_, err = m.Subscribe(t.Context(), "missing-stream", "c1", handler, factory)
	require.ErrorIs(t, err, recovery.ErrStreamNotRegistered)

	require.NoError(t, m.RegisterStream(jetstream.StreamConfig{Name: "S1", Subjects: []string{"x.>"}}))
	_, err = m.Subscribe(t.Context(), "S1", "", handler, factory)
	require.ErrorIs(t, err, recovery.ErrInvalidConsumerConfig)

	closedMgr, err := recovery.New(provider)
	require.NoError(t, err)
	closedMgr.Close()
	_, err = closedMgr.Subscribe(t.Context(), "S1", "c1", handler, factory)
	require.ErrorIs(t, err, recovery.ErrManagerClosed)
}

// TestManager_Subscribe_FactoryFailureLeavesRegistryUntouched drives
// subscriber.Subscribe into its own error path (a nil ctx) and checks that
// Manager.Subscribe neither registers the subscription nor panics.
func TestManager_Subscribe_FactoryFailureLeavesRegistryUntouched(t *testing.T) {
	t.Parallel()

	provider := newRecoveryTestProvider(t)
	m, err := recovery.New(provider)
	require.NoError(t, err)
	t.Cleanup(m.Close)

	require.NoError(t, m.RegisterStream(jetstream.StreamConfig{Name: "S1", Subjects: []string{"x.>"}}))
	factory := natsprovider.SubscriberWithConsumer(&jetstream.ConsumerConfig{Durable: "c1"})

	var nilCtx context.Context //nolint:staticcheck // deliberately nil to exercise streamSubscriber's own guard
	_, err = m.Subscribe(nilCtx, "S1", "c1", funcHandler{topic: "x.>"}, factory)
	require.Error(t, err)
	require.False(t, m.GetRegistry().HasConsumer("S1", "c1"))
}

// TestManager_Subscribe_Success exercises ManagedSubscription.setSubscriber
// and the accessor methods through the exported Manager.Subscribe surface.
func TestManager_Subscribe_Success(t *testing.T) {
	t.Parallel()

	provider := newRecoveryTestProvider(t)
	m, err := recovery.New(provider)
	require.NoError(t, err)
	t.Cleanup(m.Close)

	const streamName, consumerName, topic = "ORDERS2", "order-proc", "orders2.created"
	_, err = m.CreateStream(t.Context(), jetstream.StreamConfig{Name: streamName, Subjects: []string{"orders2.>"}})
	require.NoError(t, err)

	received := make(chan string, 1)
	handler := funcHandler{topic: topic, handle: func(_ context.Context, m *msg.Message) {
		received <- string(m.Data)
		_ = m.Ack()
	}}
	fact := natsprovider.SubscriberWithConsumer(&jetstream.ConsumerConfig{
		Durable: consumerName, FilterSubject: topic, AckPolicy: jetstream.AckExplicitPolicy,
	})

	sub, err := m.Subscribe(t.Context(), streamName, consumerName, handler, fact)
	require.NoError(t, err)

	require.Equal(t, streamName, sub.Stream())
	require.Equal(t, consumerName, sub.Consumer())
	require.Equal(t, topic, sub.Subject())
	require.False(t, sub.IsClosed())
	select {
	case <-sub.Closed():
		require.FailNow(t, "subscription must not be closed before Unsubscribe")
	default:
	}

	require.NoError(t, provider.Publish(t.Context(), *msg.NewMessage(topic, []byte("hello"))))
	select {
	case data := <-received:
		require.Equal(t, "hello", data)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "message was never delivered to the managed subscription")
	}

	sub.Unsubscribe()
	require.True(t, sub.IsClosed())
	require.False(t, m.GetRegistry().HasConsumer(streamName, consumerName))
	select {
	case <-sub.Closed():
	case <-time.After(5 * time.Second):
		require.FailNow(t, "Closed() channel must close after Unsubscribe")
	}
}

// TestAdvisoryListener_RecoversConsumerOnRealDeletion deletes a managed
// consumer directly on the server and checks that the subscription keeps
// receiving messages once the recovery supervisor recreates it.
func TestAdvisoryListener_RecoversConsumerOnRealDeletion(t *testing.T) {
	t.Parallel()

	provider := newRecoveryTestProvider(t)
	m, err := recovery.New(provider, recovery.WithRecoveryBackoff(10*time.Millisecond))
	require.NoError(t, err)
	t.Cleanup(m.Close)
	require.NoError(t, m.Start())

	const streamName, consumerName, topic = "CONSUMERS", "c-proc", "consumers.created"
	_, err = m.CreateStream(t.Context(), jetstream.StreamConfig{Name: streamName, Subjects: []string{"consumers.>"}})
	require.NoError(t, err)

	received := make(chan string, 16)
	handler := funcHandler{topic: topic, handle: func(_ context.Context, m *msg.Message) {
		received <- string(m.Data)
		_ = m.Ack()
	}}
	fact := natsprovider.SubscriberWithConsumer(&jetstream.ConsumerConfig{
		Durable: consumerName, FilterSubject: topic, AckPolicy: jetstream.AckExplicitPolicy,
	})

	sub, err := m.Subscribe(t.Context(), streamName, consumerName, handler, fact)
	require.NoError(t, err)
	t.Cleanup(sub.Unsubscribe)

	require.NoError(t, provider.Publish(t.Context(), *msg.NewMessage(topic, []byte("before"))))
	select {
	case data := <-received:
		require.Equal(t, "before", data)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "message before deletion was never delivered")
	}

	// A real server-side deletion, not a simulated advisory: the server
	// publishes the matching advisory itself.
	require.NoError(t, provider.JetStream().DeleteConsumer(t.Context(), streamName, consumerName))

	testhelpers.WaitFor(t, 5*time.Second, func() bool {
		_, err := provider.JetStream().Consumer(t.Context(), streamName, consumerName)
		return err == nil
	}, "consumer must be recreated by the recovery supervisor")

	// Drain whatever got redelivered to the fresh consumer, then prove a new
	// publish still reaches the handler.
	for drained := true; drained; {
		select {
		case <-received:
		case <-time.After(300 * time.Millisecond):
			drained = false
		}
	}

	require.NoError(t, provider.Publish(t.Context(), *msg.NewMessage(topic, []byte("after"))))
	select {
	case data := <-received:
		require.Equal(t, "after", data)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "message after recovery was never delivered")
	}
}

// --- Manager.RunHealthCheckCycle / RunStaleRecoveryCleanup ---

func TestManager_RunHealthCheckCycle(t *testing.T) {
	t.Parallel()

	t.Run("recovers_missing_stream", func(t *testing.T) {
		t.Parallel()
		provider := newRecoveryTestProvider(t)
		m, err := recovery.New(provider)
		require.NoError(t, err)
		t.Cleanup(m.Close)

		const name = "HEALTH"
		_, err = m.CreateStream(t.Context(), jetstream.StreamConfig{Name: name, Subjects: []string{"health.>"}})
		require.NoError(t, err)
		require.NoError(t, provider.JetStream().DeleteStream(t.Context(), name))

		require.NoError(t, m.RunHealthCheckCycle(t.Context()))

		testhelpers.WaitFor(t, 5*time.Second, func() bool {
			_, err := provider.JetStream().Stream(t.Context(), name)
			return err == nil
		}, "RunHealthCheckCycle must recover a missing registered stream")
	})

	t.Run("rejected_after_close", func(t *testing.T) {
		t.Parallel()
		m, err := recovery.New(newRecoveryTestProvider(t))
		require.NoError(t, err)
		m.Close()

		require.ErrorIs(t, m.RunHealthCheckCycle(t.Context()), recovery.ErrManagerClosed)
	})
}

func TestManager_RunStaleRecoveryCleanup(t *testing.T) {
	t.Parallel()

	t.Run("rejected_after_close", func(t *testing.T) {
		t.Parallel()
		m, err := recovery.New(newRecoveryTestProvider(t))
		require.NoError(t, err)
		m.Close()

		require.ErrorIs(t, m.RunStaleRecoveryCleanup(t.Context()), recovery.ErrManagerClosed)
	})

	t.Run("clears_a_mark_stuck_mid_retry", func(t *testing.T) {
		t.Parallel()
		provider := newRecoveryTestProvider(t)

		var staleCalled atomic.Bool
		m, err := recovery.New(provider,
			recovery.WithMaxRecoveryAttempts(5),
			recovery.WithRecoveryBackoff(200*time.Millisecond),
			recovery.WithStaleRecoveryTimeout(20*time.Millisecond),
			recovery.WithOnStaleRecoveryCleared(func(string) { staleCalled.Store(true) }),
		)
		require.NoError(t, err)
		t.Cleanup(m.Close)

		// A real, already-existing stream claims the subject the registered
		// "STALE" stream also wants, so every recreate attempt fails
		// deterministically with "subjects overlap", keeping the recovery
		// goroutine mid-backoff (holding its recovering mark) long enough for
		// the cleanup cycle to observe it as stale.
		_, err = provider.JetStream().CreateStream(t.Context(), jetstream.StreamConfig{Name: "BLOCKER", Subjects: []string{"dup.topic"}})
		require.NoError(t, err)
		require.NoError(t, m.RegisterStream(jetstream.StreamConfig{Name: "STALE", Subjects: []string{"dup.topic"}}))
		require.NoError(t, m.RunHealthCheckCycle(t.Context()))

		testhelpers.WaitFor(t, 2*time.Second, func() bool {
			time.Sleep(25 * time.Millisecond)
			require.NoError(t, m.RunStaleRecoveryCleanup(t.Context()))
			return staleCalled.Load()
		}, "a recovery stuck mid-backoff past the timeout must be cleared")
	})
}
