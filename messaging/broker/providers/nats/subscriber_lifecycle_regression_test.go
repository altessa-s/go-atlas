// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natsprovider

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
	"github.com/altessa-s/go-atlas/messaging/broker/msg"
)

type heldDrain struct {
	calls                   atomic.Int32
	once                    sync.Once
	entered, resume, closed chan struct{}
}

func (d *heldDrain) Stop() {}
func (d *heldDrain) Drain() {
	d.calls.Add(1)
	d.once.Do(func() { close(d.entered); go func() { <-d.resume; close(d.closed) }() })
}
func (d *heldDrain) Closed() <-chan struct{} { return d.closed }

type contextHandler struct{ seen chan error }

func (*contextHandler) Topic() string { return "orders.created" }
func (h *contextHandler) Handle(ctx context.Context, m *msg.Message) {
	h.seen <- ctx.Err()
	_ = m.Ack()
}

func TestSubscribeUnsubscribeReplacement(t *testing.T) {
	t.Parallel()
	ns := natstest.StartNATSServer(t)
	conn, js := natstest.ConnectJetStream(t, ns)
	_, err := js.CreateStream(t.Context(), jetstream.StreamConfig{Name: "ORDERS", Subjects: []string{"orders.>"}})
	require.NoError(t, err)
	provider, err := New(conn)
	require.NoError(t, err)
	ss := newStreamSubscriber(provider, nil)
	t.Cleanup(ss.Unsubscribe)
	for range 20 {
		drain := &heldDrain{entered: make(chan struct{}), resume: make(chan struct{}), closed: make(chan struct{})}
		_, cancel := context.WithCancel(t.Context())
		ss.mu.Lock()
		ss.consumeContext, ss.handlerCtxCancel = drain, cancel
		ss.mu.Unlock()
		handler := &contextHandler{seen: make(chan error, 100)}
		subscribed := make(chan error, 1)
		go func() { subscribed <- ss.Subscribe(t.Context(), handler) }()
		<-drain.entered
		unsubscribed := make(chan struct{})
		go func() { ss.Unsubscribe(); close(unsubscribed) }()
		require.Eventually(t, func() bool {
			ss.mu.Lock()
			defer ss.mu.Unlock()
			return ss.consumeContext == nil || drain.calls.Load() >= 2
		}, time.Second, time.Millisecond)
		close(drain.resume)
		if err := <-subscribed; err != nil {
			require.ErrorIs(t, err, ErrSubscribeSuperseded)
		}
		<-unsubscribed
		ss.mu.Lock()
		consumer := ss.consumeContext
		ss.mu.Unlock()
		if consumer != nil {
			select {
			case <-consumer.Closed():
				continue
			default:
			}
			_, err = js.Publish(t.Context(), handler.Topic(), []byte("test"))
			require.NoError(t, err)
			select {
			case err = <-handler.seen:
				require.NoError(t, err)
			case <-time.After(5 * time.Second):
				require.FailNow(t, "live consumer did not handle a message")
			}
		}
		ss.Unsubscribe()
	}
}
