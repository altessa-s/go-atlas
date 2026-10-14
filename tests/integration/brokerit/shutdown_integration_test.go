// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package brokerit_test

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/tests/integration/brokerit"
	"github.com/altessa-s/go-atlas/transport/broker"
	"github.com/altessa-s/go-atlas/transport/broker/msg"

	natsprovider "github.com/altessa-s/go-atlas/transport/broker/providers/nats"
)

// Unsubscribe is the graceful-shutdown path: it must cancel the handler
// context so an in-flight handler can wind down, but it must not return until
// that handler has finished — and an Ack the handler issues on its way out
// must still reach the server.
func TestShutdown_UnsubscribeWaitsForTheInFlightHandler(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	consumer := uniqueName(t)

	started := make(chan struct{})
	release := make(chan struct{})
	ctxErr := make(chan error, 1)
	rec := brokerit.NewRecorder(w.subject, func(ctx context.Context, m *msg.Message, _ int) {
		close(started)
		<-release // finishes its work regardless of the cancellation
		ctxErr <- ctx.Err()
		_ = m.Ack()
	})
	sub := w.provider.Subscriber(natsprovider.SubscriberWithConsumer(w.durable(consumer, 10*time.Second)))
	require.NoError(t, sub.Subscribe(t.Context(), rec))

	w.publish(t, "in-flight")
	<-started

	returned := make(chan struct{})
	go func() {
		defer close(returned)
		sub.Unsubscribe()
	}()

	require.Never(t, func() bool {
		select {
		case <-returned:
			return true
		default:
			return false
		}
	}, 500*time.Millisecond, samplingInterval, "Unsubscribe returned while a handler was still running")

	close(release)
	select {
	case <-returned:
	case <-time.After(settleWindow):
		t.Fatal("Unsubscribe did not return after the handler finished")
	}

	require.ErrorIs(t, <-ctxErr, context.Canceled, "Unsubscribe must cancel the handler context")
	<-sub.Closed()

	info := w.awaitSettled(t, consumer)
	require.Equal(t, uint64(1), info.Delivered.Consumer, "the in-flight message must be acknowledged, not redelivered")
	require.Equal(t, 1, rec.Count())
}

// A handler that gives up on cancellation leaves its message unacknowledged.
// It must not be lost: the next instance bound to the same durable consumer
// receives it once the ack wait expires.
func TestShutdown_AbandonedInFlightMessageGoesToTheNextInstance(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	consumer := uniqueName(t)
	cfg := w.durable(consumer, ackWait)

	started := make(chan struct{})
	first := brokerit.NewRecorder(w.subject, func(ctx context.Context, _ *msg.Message, _ int) {
		close(started)
		<-ctx.Done() // abandons the message without acknowledging it
	})
	sub := w.provider.Subscriber(natsprovider.SubscriberWithConsumer(cfg))
	require.NoError(t, sub.Subscribe(t.Context(), first))

	w.publish(t, "abandoned")
	<-started
	requireUnsubscribeReturns(t, sub.Unsubscribe)

	next := brokerit.NewRecorder(w.subject, brokerit.Ack)
	subscribe(t, newProvider(t), natsprovider.SubscriberWithConsumer(cfg), next)

	w.awaitSettled(t, consumer)
	require.Equal(t, 1, next.Attempts("abandoned"), "the abandoned message must reach the next instance")
}

// runLifecycle runs one complete subscriber lifecycle on a connection of its
// own: stream, three subscribers, a round trip through each, shutdown through
// both Unsubscribe and UnsubscribeAll, then the stream and the connection go.
// It deliberately registers no t.Cleanup, so everything it starts is stopped
// before the goroutine count is read.
func runLifecycle(t *testing.T) {
	t.Helper()

	nc, err := nats.Connect(natsURL(), nats.Timeout(2*time.Second), nats.RetryOnFailedConnect(false))
	if err != nil {
		t.Skipf("NATS unreachable at %s (%v)", natsURL(), err)
	}
	defer nc.Close()

	p, err := natsprovider.New(nc)
	require.NoError(t, err)

	name := uniqueName(t)
	subject := "brokerit." + name
	_, err = p.CreateStream(t.Context(), &jetstream.StreamConfig{Name: name, Subjects: []string{subject}, Storage: jetstream.MemoryStorage})
	require.NoError(t, err)
	defer func() { _ = p.DeleteStream(context.Background(), name) }()

	recs := make([]*brokerit.Recorder, 3)
	subs := make([]broker.Subscriber, 3)
	for i := range recs {
		recs[i] = brokerit.NewRecorder(subject, brokerit.Ack)
		subs[i] = p.Subscriber(nil)
		require.NoError(t, subs[i].Subscribe(t.Context(), recs[i]))
	}

	require.NoError(t, broker.New(p).Publish(t.Context(), *msg.NewMessage(subject, []byte("ping"))))
	for _, rec := range recs {
		require.Eventually(t, func() bool { return rec.Count() == 1 }, settleWindow, samplingInterval)
	}

	subs[0].Unsubscribe()
	p.UnsubscribeAll()
	for _, sub := range subs {
		<-sub.Closed()
	}
}

// Subscribing and shutting down must not leak goroutines: a service that
// re-subscribes on every config reload or reconnect would otherwise grow
// without bound. Several full lifecycles run back to back, and the goroutine
// count must come back to where it started.
//
// Serial on purpose: runtime.NumGoroutine is process-wide, and parallel tests
// starting or stopping subscribers would move it. Top-level parallel tests
// stay paused until the serial ones have finished, so this one reads a quiet
// process.
func TestShutdown_SubscriberLifecycleLeaksNoGoroutines(t *testing.T) {
	// Warm up once so lazily started runtime and library goroutines are part
	// of the baseline rather than counted as a leak.
	runLifecycle(t)
	baseline := settledGoroutines()

	const cycles = 5
	for range cycles {
		runLifecycle(t)
	}

	deadline := time.Now().Add(settleWindow)
	n := runtime.NumGoroutine()
	for n > baseline && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		n = runtime.NumGoroutine()
	}

	if n > baseline {
		buf := make([]byte, 1<<20)
		t.Fatalf("goroutines leaked: %d before %d lifecycles, %d after\n%s", baseline, cycles, n, buf[:runtime.Stack(buf, true)])
	}
}

// settledGoroutines returns the goroutine count once it has stopped falling,
// so goroutines still winding down from a closed connection are not part of
// the baseline.
func settledGoroutines() int {
	n := runtime.NumGoroutine()
	for range 40 {
		time.Sleep(50 * time.Millisecond)
		next := runtime.NumGoroutine()
		if next >= n {
			return n
		}
		n = next
	}

	return n
}
