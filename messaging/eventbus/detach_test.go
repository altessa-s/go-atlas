// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package eventbus_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/messaging/eventbus"
)

type detachCtxKey struct{}

func TestDetachStartsAFreshDispatchChain(t *testing.T) {
	t.Parallel()

	// A goroutine the handler starts publishes the handler's own event type
	// while that dispatch is still running: a cycle with the handler's
	// context, a fresh chain with the detached one.
	bus := eventbus.New()
	var followUps int
	eventbus.Subscribe(bus, func(ctx context.Context, e *eventA) error {
		if e.n != 0 {
			followUps++
			require.Equal(t, "kept", ctx.Value(detachCtxKey{}), "Detach must keep the other context values")
			return nil
		}
		nested := make(chan error)
		detached := make(chan error)
		go func() {
			nested <- eventbus.Publish(ctx, bus, &eventA{n: 1})
			detached <- eventbus.Publish(eventbus.Detach(ctx), bus, &eventA{n: 1})
		}()
		require.ErrorIs(t, <-nested, eventbus.ErrEventCycle)
		require.NoError(t, <-detached)
		return nil
	})

	ctx := context.WithValue(t.Context(), detachCtxKey{}, "kept")
	require.NoError(t, eventbus.Publish(ctx, bus, &eventA{}))
	require.Equal(t, 1, followUps)
}

func TestDetachOutsideADispatchIsHarmless(t *testing.T) {
	t.Parallel()

	bus := eventbus.New()
	var calls int
	eventbus.Subscribe(bus, func(context.Context, *eventA) error { calls++; return nil })

	require.NoError(t, eventbus.Publish(eventbus.Detach(t.Context()), bus, &eventA{}))
	require.Equal(t, 1, calls)
}

func TestPublishNilEventCycleIsRejectedAtOnce(t *testing.T) {
	t.Parallel()

	// A nil event runs the global adapters only. One that publishes nil again
	// re-enters itself: a cycle, not a recursion to the depth limit.
	bus := eventbus.New()
	var calls int
	bus.RegisterGlobalAdapter(func(ctx context.Context, event any) error {
		calls++
		if event != nil {
			return nil
		}
		return bus.Publish(ctx, nil)
	})

	err := bus.Publish(t.Context(), nil)
	require.ErrorIs(t, err, eventbus.ErrEventCycle)
	require.NotErrorIs(t, err, eventbus.ErrDispatchTooDeep)
	require.Equal(t, 1, calls)

}

func TestPublishNilEventUnderATypedOneIsAllowed(t *testing.T) {
	t.Parallel()

	bus := eventbus.New()
	var nilEvents int
	bus.RegisterGlobalAdapter(func(_ context.Context, event any) error {
		if event == nil {
			nilEvents++
		}
		return nil
	})
	eventbus.Subscribe(bus, func(ctx context.Context, _ *eventA) error { return bus.Publish(ctx, nil) })

	require.NoError(t, eventbus.Publish(t.Context(), bus, &eventA{}))
	require.Equal(t, 1, nilEvents)
}
