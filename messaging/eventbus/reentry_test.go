// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package eventbus_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/messaging/eventbus"
)

func TestPublishSelfCycleIsRejected(t *testing.T) {
	t.Parallel()

	bus := eventbus.New()
	eventbus.Subscribe(bus, func(ctx context.Context, _ *eventA) error {
		return eventbus.Publish(ctx, bus, &eventA{}) // re-publishes the in-flight type
	})

	err := eventbus.Publish(t.Context(), bus, &eventA{})
	require.ErrorIs(t, err, eventbus.ErrEventCycle)
}

func TestPublishMutualCycleIsRejected(t *testing.T) {
	t.Parallel()

	bus := eventbus.New()
	eventbus.Subscribe(bus, func(ctx context.Context, _ *eventA) error {
		return eventbus.Publish(ctx, bus, &eventB{})
	})
	eventbus.Subscribe(bus, func(ctx context.Context, _ *eventB) error {
		return eventbus.Publish(ctx, bus, &eventA{}) // A is already in flight → cycle
	})

	err := eventbus.Publish(t.Context(), bus, &eventA{})
	require.ErrorIs(t, err, eventbus.ErrEventCycle)
}

func TestPublishLinearCascadeOfDistinctTypesIsAllowed(t *testing.T) {
	t.Parallel()

	bus := eventbus.New()
	var ranB bool
	eventbus.Subscribe(bus, func(ctx context.Context, _ *eventA) error {
		return eventbus.Publish(ctx, bus, &eventB{})
	})
	eventbus.Subscribe(bus, func(_ context.Context, _ *eventB) error {
		ranB = true
		return nil
	})

	require.NoError(t, eventbus.Publish(t.Context(), bus, &eventA{}))
	require.True(t, ranB, "downstream cascade handler must run")
}

func TestPublishConcurrentlyFromOneHandlerIsAllowed(t *testing.T) {
	t.Parallel()

	// Publishes of one type from goroutines sharing the handler's context are
	// siblings, not a cycle, and must not race on the chain state. Every
	// eventB handler stays inside its dispatch until all of them are, so the
	// publishes overlap in full while eventA is still being dispatched.
	const publishers = 64
	bus := eventbus.New()
	var inB atomic.Int32
	allInB := make(chan struct{})
	eventbus.Subscribe(bus, func(ctx context.Context, _ *eventA) error {
		errs := make([]error, publishers)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := range publishers {
			wg.Go(func() {
				<-start
				errs[i] = eventbus.Publish(ctx, bus, &eventB{})
			})
		}
		close(start)
		wg.Wait()
		return errors.Join(errs...)
	})
	eventbus.Subscribe(bus, func(ctx context.Context, _ *eventB) error {
		if inB.Add(1) == publishers {
			close(allInB)
		}
		select {
		case <-allInB:
			return nil
		case <-time.After(10 * time.Second):
			return errors.New("the sibling publishes did not overlap")
		case <-ctx.Done():
			return ctx.Err()
		}
	})

	require.NoError(t, eventbus.Publish(t.Context(), bus, &eventA{}))
	require.EqualValues(t, publishers, inB.Load())
}

func TestPublishCycleFromAGoroutineIsRejected(t *testing.T) {
	t.Parallel()

	// While eventA is being dispatched, goroutines sharing its context publish
	// together: eventA again is a cycle for each of them, eventB is not.
	const publishers = 32
	bus := eventbus.New()
	cycleErrs := make([]error, publishers)
	siblingErrs := make([]error, publishers)
	var ranB atomic.Int32
	eventbus.Subscribe(bus, func(ctx context.Context, _ *eventA) error {
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := range publishers {
			wg.Go(func() {
				<-start
				cycleErrs[i] = eventbus.Publish(ctx, bus, &eventA{})
			})
			wg.Go(func() {
				<-start
				siblingErrs[i] = eventbus.Publish(ctx, bus, &eventB{})
			})
		}
		close(start)
		wg.Wait()
		return nil
	})
	eventbus.Subscribe(bus, func(_ context.Context, _ *eventB) error {
		ranB.Add(1)
		return nil
	})

	require.NoError(t, eventbus.Publish(t.Context(), bus, &eventA{}))
	for i := range publishers {
		require.ErrorIs(t, cycleErrs[i], eventbus.ErrEventCycle)
		require.NoError(t, siblingErrs[i])
	}
	require.EqualValues(t, publishers, ranB.Load())
}

func TestPublishSameTypeSequentiallyIsAllowed(t *testing.T) {
	t.Parallel()

	// Re-publishing the same type after its dispatch completed is not a cycle:
	// the type leaves the in-flight set when its dispatch returns.
	bus := eventbus.New()
	var calls int
	eventbus.Subscribe(bus, func(_ context.Context, _ *eventA) error { calls++; return nil })

	require.NoError(t, eventbus.Publish(t.Context(), bus, &eventA{}))
	require.NoError(t, eventbus.Publish(t.Context(), bus, &eventA{}))
	require.Equal(t, 2, calls)
}
