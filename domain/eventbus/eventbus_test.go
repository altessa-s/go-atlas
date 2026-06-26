// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package eventbus_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/eventbus"
)

// eventA and eventB are distinct event types used to exercise type routing.
type eventA struct{ n int }

type eventB struct{ s string }

func TestPublishRunsHandlersInRegistrationOrderThenAdaptersThenGlobals(t *testing.T) {
	t.Parallel()

	bus := eventbus.New()
	var order []string
	record := func(label string) eventbus.Handler {
		return func(_ context.Context, _ any) error {
			order = append(order, label)
			return nil
		}
	}

	bus.Subscribe(&eventA{}, record("handler-1"))
	bus.Subscribe(&eventA{}, record("handler-2"))
	bus.RegisterAdapter(&eventA{}, record("adapter-1"))
	bus.RegisterGlobalAdapter(record("global-1"))

	require.NoError(t, bus.Publish(t.Context(), &eventA{}))
	require.Equal(t, []string{"handler-1", "handler-2", "adapter-1", "global-1"}, order)
}

func TestPublishStopsAtFirstHandlerErrorAndWraps(t *testing.T) {
	t.Parallel()

	bus := eventbus.New()
	sentinel := errors.New("veto")
	var reached bool

	bus.Subscribe(&eventA{}, func(_ context.Context, _ any) error { return sentinel })
	bus.Subscribe(&eventA{}, func(_ context.Context, _ any) error { reached = true; return nil })

	err := bus.Publish(t.Context(), &eventA{})
	require.ErrorIs(t, err, sentinel)
	require.False(t, reached, "second handler must not run after the first errors")
}

func TestPublishOnlyDeliversToMatchingType(t *testing.T) {
	t.Parallel()

	bus := eventbus.New()
	var gotA, gotB int
	bus.Subscribe(&eventA{}, func(_ context.Context, _ any) error { gotA++; return nil })
	bus.Subscribe(&eventB{}, func(_ context.Context, _ any) error { gotB++; return nil })

	require.NoError(t, bus.Publish(t.Context(), &eventA{}))
	require.Equal(t, 1, gotA)
	require.Equal(t, 0, gotB)
}

func TestPublishNilEventRunsOnlyGlobalAdapters(t *testing.T) {
	t.Parallel()

	bus := eventbus.New()
	var typed, global int
	bus.Subscribe(&eventA{}, func(_ context.Context, _ any) error { typed++; return nil })
	bus.RegisterGlobalAdapter(func(_ context.Context, _ any) error { global++; return nil })

	require.NoError(t, bus.Publish(t.Context(), nil))
	require.Equal(t, 0, typed)
	require.Equal(t, 1, global)
}

func TestPublishWithNoSubscribersIsNoError(t *testing.T) {
	t.Parallel()

	bus := eventbus.New()
	require.NoError(t, bus.Publish(t.Context(), &eventA{}))
}

func TestConcurrentSubscribeAndPublish(t *testing.T) {
	t.Parallel()

	bus := eventbus.New()
	bus.Subscribe(&eventA{}, func(_ context.Context, _ any) error { return nil })

	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() { bus.Subscribe(&eventA{}, func(_ context.Context, _ any) error { return nil }) })
		wg.Go(func() { _ = bus.Publish(t.Context(), &eventA{}) })
	}
	wg.Wait()
}
