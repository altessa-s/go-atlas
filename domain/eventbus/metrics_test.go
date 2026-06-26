// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package eventbus_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/eventbus"
	"github.com/altessa-s/go-atlas/observability/metrics"
)

func TestObservedBusForwardsSubscribeAndPublish(t *testing.T) {
	t.Parallel()

	observed := eventbus.NewObserved(eventbus.New(), metrics.Noop())
	var calls int
	observed.Subscribe(&eventA{}, func(_ context.Context, _ any) error { calls++; return nil })

	require.NoError(t, observed.Publish(t.Context(), &eventA{}))
	require.Equal(t, 1, calls)
}

func TestObservedBusPropagatesError(t *testing.T) {
	t.Parallel()

	observed := eventbus.NewObserved(eventbus.New(), metrics.Noop())
	sentinel := errors.New("boom")
	observed.Subscribe(&eventA{}, func(_ context.Context, _ any) error { return sentinel })

	require.ErrorIs(t, observed.Publish(t.Context(), &eventA{}), sentinel)
}

func TestObservedBusNilCollectorIsSafe(t *testing.T) {
	t.Parallel()

	observed := eventbus.NewObserved(eventbus.New(), nil)
	require.NoError(t, observed.Publish(t.Context(), &eventA{}))
}

func TestObservedBusNoHandlerForRequiredTypeIsSafe(t *testing.T) {
	t.Parallel()

	// A required event type with no handler must still publish without error;
	// the decorator only records the no-handler metric and logs a warning.
	bus := eventbus.New()
	eventbus.Require[*eventA](bus)
	observed := eventbus.NewObserved(bus, metrics.Noop())

	require.NoError(t, observed.Publish(t.Context(), &eventA{}))
}

func TestTypedHelpersWorkThroughObservedBus(t *testing.T) {
	t.Parallel()

	observed := eventbus.NewObserved(eventbus.New(), metrics.Noop())
	var calls int
	eventbus.Subscribe(observed, func(_ context.Context, _ *eventA) error { calls++; return nil })

	require.NoError(t, eventbus.Publish(t.Context(), observed, &eventA{}))
	require.Equal(t, 1, calls)
}
