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
)

func TestTypedSubscribeReceivesTypedEventWithoutAssertion(t *testing.T) {
	t.Parallel()

	bus := eventbus.New()
	var got *eventA
	eventbus.Subscribe(bus, func(_ context.Context, e *eventA) error {
		got = e
		return nil
	})

	require.NoError(t, eventbus.Publish(t.Context(), bus, &eventA{n: 7}))
	require.NotNil(t, got)
	require.Equal(t, 7, got.n)
}

func TestTypedPublishReachesTypedSubscriberSameKey(t *testing.T) {
	t.Parallel()

	bus := eventbus.New()
	var calls int
	// Subscribe via the typed helper, publish via the untyped Publish: both must
	// key on *eventA so the handler still fires (pointer/value consistency).
	eventbus.Subscribe(bus, func(_ context.Context, _ *eventA) error { calls++; return nil })

	require.NoError(t, bus.Publish(t.Context(), &eventA{}))
	require.NoError(t, eventbus.Publish(t.Context(), bus, &eventA{}))
	require.Equal(t, 2, calls)
}

func TestTypedSubscribeIgnoresOtherTypes(t *testing.T) {
	t.Parallel()

	bus := eventbus.New()
	var calls int
	eventbus.Subscribe(bus, func(_ context.Context, _ *eventA) error { calls++; return nil })

	require.NoError(t, eventbus.Publish(t.Context(), bus, &eventB{}))
	require.Equal(t, 0, calls)
}

func TestTypedHandlerErrorPropagates(t *testing.T) {
	t.Parallel()

	bus := eventbus.New()
	sentinel := errors.New("typed veto")
	eventbus.Subscribe(bus, func(_ context.Context, _ *eventA) error { return sentinel })

	require.ErrorIs(t, eventbus.Publish(t.Context(), bus, &eventA{}), sentinel)
}

func TestTypedRegisterAdapterRunsAfterHandlers(t *testing.T) {
	t.Parallel()

	bus := eventbus.New()
	var order []string
	eventbus.Subscribe(bus, func(_ context.Context, _ *eventA) error { order = append(order, "handler"); return nil })
	eventbus.RegisterAdapter(bus, func(_ context.Context, _ *eventA) error { order = append(order, "adapter"); return nil })

	require.NoError(t, eventbus.Publish(t.Context(), bus, &eventA{}))
	require.Equal(t, []string{"handler", "adapter"}, order)
}
