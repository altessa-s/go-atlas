// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package eventbus_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/eventbus"
	"github.com/altessa-s/go-atlas/observability/metrics"
)

func alwaysInTx(context.Context) bool { return true }
func neverInTx(context.Context) bool  { return false }

func TestPublishTxDeliversWhenInTransaction(t *testing.T) {
	t.Parallel()

	bus := eventbus.New(eventbus.WithTxProbe(alwaysInTx))
	var calls int
	eventbus.Subscribe(bus, func(context.Context, *eventA) error { calls++; return nil })

	require.NoError(t, eventbus.PublishTx(t.Context(), bus, &eventA{}))
	require.Equal(t, 1, calls)
}

func TestPublishTxFailsWhenNotInTransaction(t *testing.T) {
	t.Parallel()

	bus := eventbus.New(eventbus.WithTxProbe(neverInTx))
	var calls int
	eventbus.Subscribe(bus, func(context.Context, *eventA) error { calls++; return nil })

	err := eventbus.PublishTx(t.Context(), bus, &eventA{})
	require.ErrorIs(t, err, eventbus.ErrNotInTransaction)
	require.Zero(t, calls, "handler must not run when not in a transaction")
}

func TestPublishTxFailsWhenNoProbeConfigured(t *testing.T) {
	t.Parallel()

	bus := eventbus.New() // no WithTxProbe → InTransaction is always false
	require.False(t, bus.InTransaction(t.Context()))
	require.ErrorIs(t, eventbus.PublishTx(t.Context(), bus, &eventA{}), eventbus.ErrNotInTransaction)
}

func TestAnyInTx(t *testing.T) {
	t.Parallel()

	require.False(t, eventbus.AnyInTx()(t.Context()), "no probes → false")
	require.False(t, eventbus.AnyInTx(neverInTx, nil)(t.Context()), "all false / nil skipped")
	require.True(t, eventbus.AnyInTx(neverInTx, nil, alwaysInTx)(t.Context()), "any true → true")
}

func TestPublishTxThroughObservedBus(t *testing.T) {
	t.Parallel()

	observed := eventbus.NewObserved(eventbus.New(eventbus.WithTxProbe(alwaysInTx)), metrics.Noop())
	var calls int
	eventbus.Subscribe(observed, func(context.Context, *eventA) error { calls++; return nil })

	require.True(t, observed.InTransaction(t.Context()), "InTransaction must forward through the decorator")
	require.NoError(t, eventbus.PublishTx(t.Context(), observed, &eventA{}))
	require.Equal(t, 1, calls)
}
