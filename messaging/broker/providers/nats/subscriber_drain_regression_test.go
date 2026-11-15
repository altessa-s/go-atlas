// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natsprovider

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
	"github.com/altessa-s/go-atlas/messaging/broker/msg"
)

// drainingConsumeContext emulates a JetStream consume context whose Drain
// runs one buffered callback asynchronously and then reports closed.
type drainingConsumeContext struct {
	closed   chan struct{}
	callback func()
}

func (c *drainingConsumeContext) Stop() {}

func (c *drainingConsumeContext) Drain() {
	go func() {
		c.callback()
		close(c.closed)
	}()
}

func (c *drainingConsumeContext) Closed() <-chan struct{} { return c.closed }

// TestStreamSubscriber_UnsubscribeDrainsBeforeCancel pins that a message still
// buffered when Unsubscribe starts is handled with a live context.
func TestStreamSubscriber_UnsubscribeDrainsBeforeCancel(t *testing.T) {
	t.Parallel()

	handlerCtx, cancel := context.WithCancel(t.Context())
	ss := &streamSubscriber{drainTimeout: 30 * time.Second}

	var seen error
	ss.consumeContext = &drainingConsumeContext{closed: make(chan struct{}), callback: func() {
		ss.handlersWg.Add(1)
		defer ss.handlersWg.Done()
		time.Sleep(50 * time.Millisecond) // the handler is still running when Drain returns
		seen = handlerCtx.Err()
	}}
	ss.handlerCtxCancel = cancel

	ss.Unsubscribe()

	require.NoError(t, seen, "a buffered message was handled with a canceled context")
	require.ErrorIs(t, handlerCtx.Err(), context.Canceled, "Unsubscribe must cancel the handler context in the end")
}

// TestStreamSubscriber_UnsubscribeGraceIsBounded pins that a handler outliving
// the drain timeout is canceled rather than waited for indefinitely.
func TestStreamSubscriber_UnsubscribeGraceIsBounded(t *testing.T) {
	t.Parallel()

	const grace = 200 * time.Millisecond

	handlerCtx, cancel := context.WithCancel(t.Context())
	ss := &streamSubscriber{drainTimeout: grace}

	ss.consumeContext = &drainingConsumeContext{closed: make(chan struct{}), callback: func() {
		ss.handlersWg.Add(1)
		defer ss.handlersWg.Done()
		<-handlerCtx.Done()
	}}
	ss.handlerCtxCancel = cancel

	started := time.Now()
	done := make(chan struct{})
	go func() {
		defer close(done)
		ss.Unsubscribe()
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		require.FailNow(t, "Unsubscribe did not return after the drain timeout")
	}
	elapsed := time.Since(started)
	require.GreaterOrEqual(t, elapsed, grace, "the handler context was canceled before the grace period ended")
	require.Less(t, elapsed, 5*time.Second, "the grace period did not bound Unsubscribe")
}

type topicHandler string

func (topicHandler) Handle(context.Context, *msg.Message) {}
func (h topicHandler) Topic() string                      { return string(h) }

// TestStreamSubscriber_ResubscribeStopsPreviousConsume pins that a second
// Subscribe does not leave the first Consume pulling messages.
func TestStreamSubscriber_ResubscribeStopsPreviousConsume(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	conn, js := natstest.ConnectJetStream(t, ns)
	_, err := js.CreateStream(t.Context(), jetstream.StreamConfig{Name: "ORDERS", Subjects: []string{"orders.>"}})
	require.NoError(t, err)

	provider, err := New(conn)
	require.NoError(t, err)

	sub := SubscriberWithEphemeralConsumer()(provider)
	ss, ok := sub.(*streamSubscriber)
	require.True(t, ok)
	t.Cleanup(sub.Unsubscribe)

	require.NoError(t, sub.Subscribe(t.Context(), topicHandler("orders.created")))
	ss.mu.Lock()
	first := ss.consumeContext
	ss.mu.Unlock()

	require.NoError(t, sub.Subscribe(t.Context(), topicHandler("orders.created")))
	ss.mu.Lock()
	second := ss.consumeContext
	ss.mu.Unlock()

	require.NotSame(t, first, second)
	select {
	case <-first.Closed():
	default:
		require.FailNow(t, "the first consume is still running after the second Subscribe")
	}
	select {
	case <-second.Closed():
		require.FailNow(t, "the second consume must be running")
	default:
	}
}
