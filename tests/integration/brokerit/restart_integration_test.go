// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package brokerit_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/tests/integration/brokerit"
	"github.com/altessa-s/go-atlas/transport/broker/msg"

	natsprovider "github.com/altessa-s/go-atlas/transport/broker/providers/nats"
)

// ackLog records the payloads whose Ack was accepted by the client.
type ackLog struct {
	mu    sync.Mutex
	acked map[string]int
}

func newAckLog() *ackLog { return &ackLog{acked: make(map[string]int)} }

func (l *ackLog) ack(m *msg.Message) {
	if m.Ack() != nil {
		return
	}
	l.mu.Lock()
	l.acked[string(m.Data)]++
	l.mu.Unlock()
}

func (l *ackLog) has(payload string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.acked[payload] > 0
}

// slowWork is how long a first-instance handler takes per message. It keeps
// the stream from being drained before the instance is stopped, so the stop
// really lands mid-stream.
const slowWork = 5 * time.Millisecond

// smallBuffer bounds how many messages an instance pulls ahead of its handler,
// so a stopped instance leaves most of the stream to its successor.
var smallBuffer = jetstream.PullMaxMessages(10)

// requireUnsubscribeReturns runs Unsubscribe and fails if it does not return
// within the settle window — a subscriber whose shutdown hangs would block
// the process it runs in.
func requireUnsubscribeReturns(tb testing.TB, unsubscribe func()) {
	tb.Helper()

	done := make(chan struct{})
	go func() {
		defer close(done)
		unsubscribe()
	}()

	select {
	case <-done:
	case <-time.After(settleWindow):
		tb.Fatal("Unsubscribe did not return")
	}
}

// A consumer instance dies mid-stream — its connection drops while messages
// are buffered, some handled and acknowledged, some handled and never
// acknowledged. A successor bound to the same durable consumer must receive
// everything the first one did not acknowledge: across the two instances no
// message is lost. Duplicates are allowed (an ack in flight when the
// connection dropped) and are counted, not forbidden.
func TestRestart_NothingLostWhenAConsumerDiesMidStream(t *testing.T) {
	t.Parallel()

	const total, dieAfter = 200, 60

	w := newWorld(t)
	consumer := uniqueName(t)
	cfg := w.durable(consumer, ackWait)

	// The first instance rides a connection of its own so the test can sever it.
	first := newProvider(t)
	var dead atomic.Bool
	firstLog := newAckLog()
	firstRec := brokerit.NewRecorder(w.subject, func(_ context.Context, m *msg.Message, _ int) {
		time.Sleep(slowWork)
		if dead.Load() {
			return // crashed between receiving the message and acknowledging it
		}
		firstLog.ack(m)
	})
	firstSub := first.Subscriber(natsprovider.SubscriberWithConsumer(cfg, smallBuffer))
	require.NoError(t, firstSub.Subscribe(t.Context(), firstRec))

	want := payloads("restart", total)
	w.publish(t, want...)

	require.Eventually(t, func() bool { return firstRec.Count() >= dieAfter }, settleWindow, samplingInterval)
	dead.Store(true)
	first.NATSConn().Close()

	// Shutting down a subscriber whose connection is already gone must still
	// return: a process tearing down after a broker outage calls exactly this.
	requireUnsubscribeReturns(t, firstSub.Unsubscribe)

	second := newProvider(t)
	secondRec := brokerit.NewRecorder(w.subject, brokerit.Ack)
	subscribe(t, second, natsprovider.SubscriberWithConsumer(cfg), secondRec)

	w.awaitSettled(t, consumer)

	var duplicates int
	for _, p := range want {
		byFirst, bySecond := firstLog.has(p), secondRec.Attempts(p) > 0
		require.True(t, byFirst || bySecond, "message %s was lost across the restart", p)
		if byFirst && bySecond {
			duplicates++
		}
	}
	require.Positive(t, secondRec.Count(), "the successor must have taken over part of the stream")
	t.Logf("first instance received %d deliveries before dying, the successor processed %d, %d duplicated",
		firstRec.Count(), secondRec.Count(), duplicates)
}

// A graceful handover: the first instance unsubscribes mid-stream — draining
// what it has buffered and acknowledging it — and a successor picks up the
// rest. Nothing is lost, and since every message the first instance handled
// was acknowledged, nothing is processed twice.
func TestRestart_GracefulHandoverLosesAndDuplicatesNothing(t *testing.T) {
	t.Parallel()

	const total, stopAfter = 200, 60

	w := newWorld(t)
	consumer := uniqueName(t)
	cfg := w.durable(consumer, ackWait)

	first := newProvider(t)
	firstRec := brokerit.NewRecorder(w.subject, func(ctx context.Context, m *msg.Message, attempt int) {
		time.Sleep(slowWork)
		brokerit.Ack(ctx, m, attempt)
	})
	firstSub := first.Subscriber(natsprovider.SubscriberWithConsumer(cfg, smallBuffer))
	require.NoError(t, firstSub.Subscribe(t.Context(), firstRec))

	want := payloads("handover", total)
	w.publish(t, want...)

	require.Eventually(t, func() bool { return firstRec.Count() >= stopAfter }, settleWindow, samplingInterval)
	requireUnsubscribeReturns(t, firstSub.Unsubscribe)

	second := newProvider(t)
	secondRec := brokerit.NewRecorder(w.subject, brokerit.Ack)
	subscribe(t, second, natsprovider.SubscriberWithConsumer(cfg), secondRec)

	w.awaitSettled(t, consumer)

	require.Positive(t, secondRec.Count(), "the successor must have taken over part of the stream")
	for _, p := range want {
		n := firstRec.Attempts(p) + secondRec.Attempts(p)
		require.Equal(t, 1, n, "message %s processed %d times across the handover", p, n)
	}
}
