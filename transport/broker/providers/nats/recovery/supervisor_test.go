// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/broker"
)

// TestSupervisor_ClearStaleRecoveries_CallbackMayReenter is the regression
// guard for OnStaleCleared running under recoveringMu: a callback calling back
// into the supervisor (here GetRecoveringStreams) deadlocked on that mutex.
func TestSupervisor_ClearStaleRecoveries_CallbackMayReenter(t *testing.T) {
	t.Parallel()

	var sup *Supervisor
	var seen []string
	callbackRan := false
	sup = NewSupervisor(SupervisorConfig{
		Registry: NewRegistry(),
		Logger:   slog.New(slog.DiscardHandler),
		OnStaleCleared: func(string) {
			callbackRan = true
			seen = sup.GetRecoveringStreams()
		},
	})
	t.Cleanup(sup.Close)

	sup.recoveringStates["stale"] = &recoveryState{startedAt: time.Now().Add(-time.Hour)}

	done := make(chan []string, 1)
	go func() { done <- sup.ClearStaleRecoveries(time.Minute) }()

	select {
	case cleared := <-done:
		require.Equal(t, []string{"stale"}, cleared)
		require.True(t, callbackRan)
		require.Empty(t, seen, "the cleared mark must be gone when the callback runs")
	case <-time.After(5 * time.Second):
		t.Fatal("ClearStaleRecoveries deadlocked: OnStaleCleared ran under recoveringMu")
	}
}

// ctxSubscriber is a broker.Subscriber whose Subscribe hands its context to the
// test through subscribed and then, when block is set, waits for that context
// to be canceled.
type ctxSubscriber struct {
	subscribed chan context.Context
	block      bool
}

func (s *ctxSubscriber) Subscribe(ctx context.Context, _ broker.SubscriberHandler) error {
	s.subscribed <- ctx
	if s.block {
		<-ctx.Done()
		return ctx.Err()
	}
	return nil
}

func (s *ctxSubscriber) Unsubscribe() {}

func (s *ctxSubscriber) Closed() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

// newTestManagedSubscription wires a ManagedSubscription into registry the
// same way Manager.Subscribe does, with parent as the original Subscribe
// context and sub produced by the factory.
func newTestManagedSubscription(parent context.Context, registry *Registry, stream, consumer string, sub broker.Subscriber) *ManagedSubscription {
	ms := &ManagedSubscription{
		manager:      &Manager{registry: registry},
		stream:       stream,
		consumerName: consumer,
		factory:      func(any) broker.Subscriber { return sub },
	}
	registry.RegisterStream(stream, jetstream.StreamConfig{Name: stream}, RecoveryStrategyAuto)
	registry.RegisterResubscribeHandler(stream, consumer, func(recoveryCtx context.Context) error {
		return ms.resubscribe(parent, recoveryCtx)
	})
	return ms
}

// TestSupervisor_Close_CancelsResubscribe is the regression guard for
// resubscribe handlers capturing the original Subscribe context: Close canceled
// only the supervisor's own context, so a Subscribe blocked on its context kept
// the recovery goroutine, and therefore Close, waiting forever.
func TestSupervisor_Close_CancelsResubscribe(t *testing.T) {
	t.Parallel()

	parent, cancelParent := context.WithCancel(t.Context())
	defer cancelParent()

	registry := NewRegistry()
	sub := &ctxSubscriber{subscribed: make(chan context.Context, 1), block: true}
	newTestManagedSubscription(parent, registry, "stream-1", "consumer-1", sub)

	sup := NewSupervisor(SupervisorConfig{
		Registry:    registry,
		Logger:      slog.New(slog.DiscardHandler),
		MaxAttempts: 1,
		Backoff:     time.Millisecond,
	})
	sup.RecoverConsumer("stream-1", "consumer-1", nil)

	var subCtx context.Context
	select {
	case subCtx = <-sub.subscribed:
	case <-time.After(5 * time.Second):
		t.Fatal("recovery never reached Subscribe")
	}

	closed := make(chan struct{})
	go func() {
		sup.Close()
		close(closed)
	}()

	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not interrupt the blocked resubscribe")
	}
	require.ErrorIs(t, subCtx.Err(), context.Canceled)
	require.NoError(t, parent.Err(), "the original subscription context must not be canceled")
}

// TestManagedSubscription_ResubscribeOutlivesRecoveryContext checks that the
// recovery context bounds only establishment: once Subscribe succeeded, ending
// recovery leaves the new subscription's context alive, and Unsubscribe is what
// cancels it.
func TestManagedSubscription_ResubscribeOutlivesRecoveryContext(t *testing.T) {
	t.Parallel()

	type ctxKey struct{}
	parent := context.WithValue(t.Context(), ctxKey{}, "kept")

	registry := NewRegistry()
	sub := &ctxSubscriber{subscribed: make(chan context.Context, 1)}
	ms := newTestManagedSubscription(parent, registry, "stream-1", "consumer-1", sub)

	recoveryCtx, endRecovery := context.WithCancel(t.Context())
	require.NoError(t, ms.resubscribe(parent, recoveryCtx))
	endRecovery()

	subCtx := <-sub.subscribed
	require.NoError(t, subCtx.Err(), "ending recovery must not cancel an established subscription")
	require.Equal(t, "kept", subCtx.Value(ctxKey{}), "the subscription context must keep the original values")

	ms.Unsubscribe()
	require.ErrorIs(t, subCtx.Err(), context.Canceled)
}
