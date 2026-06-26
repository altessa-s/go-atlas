// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package eventbus_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/eventbus"
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
