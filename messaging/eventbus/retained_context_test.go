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

func TestPublishWithRetainedContextAfterDispatchIsAllowed(t *testing.T) {
	t.Parallel()

	// A handler keeps the context of each dispatch and the next publish reuses
	// it. A dispatch that has returned is neither in flight nor counted toward
	// the depth, however many of them the retained context has been through.
	const rounds = 100 // far past the depth limit
	bus := eventbus.New()
	retained := t.Context()
	var calls int
	eventbus.Subscribe(bus, func(ctx context.Context, _ *eventA) error {
		retained = context.WithoutCancel(ctx)
		calls++
		return nil
	})

	for range rounds {
		require.NoError(t, eventbus.Publish(retained, bus, &eventA{}))
	}
	require.Equal(t, rounds, calls)
}

func TestPublishFromAGoroutineOutlivingItsDispatch(t *testing.T) {
	t.Parallel()

	bus := eventbus.New()
	duringDispatch := make(chan error)
	dispatchReturned := make(chan struct{})
	afterDispatch := make(chan error, 1)
	eventbus.Subscribe(bus, func(ctx context.Context, e *eventA) error {
		if e.n != 0 {
			return nil // the re-publish from the goroutine
		}
		go func() {
			duringDispatch <- eventbus.Publish(ctx, bus, &eventA{n: 1})
			<-dispatchReturned
			afterDispatch <- eventbus.Publish(ctx, bus, &eventA{n: 1})
		}()
		// The handler is still inside the dispatch while the goroutine
		// publishes for the first time.
		require.ErrorIs(t, <-duringDispatch, eventbus.ErrEventCycle)
		return nil
	})

	require.NoError(t, eventbus.Publish(t.Context(), bus, &eventA{}))
	close(dispatchReturned)
	require.NoError(t, <-afterDispatch, "the dispatch has returned: publishing its type again is not a cycle")
}

func TestPublishWithRetainedContextStillSeesRunningAncestors(t *testing.T) {
	t.Parallel()

	// eventA's handler publishes eventB and keeps the context eventB was
	// dispatched with. Once that dispatch has returned eventB may be published
	// again, but eventA is still running, so it stays a cycle.
	bus := eventbus.New()
	var bCtx context.Context
	var calls int
	eventbus.Subscribe(bus, func(ctx context.Context, _ *eventB) error {
		bCtx = ctx
		calls++
		return nil
	})
	eventbus.Subscribe(bus, func(ctx context.Context, _ *eventA) error {
		require.NoError(t, eventbus.Publish(ctx, bus, &eventB{}))
		require.NoError(t, eventbus.Publish(bCtx, bus, &eventB{}))
		require.ErrorIs(t, eventbus.Publish(bCtx, bus, &eventA{}), eventbus.ErrEventCycle)
		return nil
	})

	require.NoError(t, eventbus.Publish(t.Context(), bus, &eventA{}))
	require.Equal(t, 2, calls)
}
