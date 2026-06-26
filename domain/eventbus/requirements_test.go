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

func TestValidatePassesWhenRequiredTypeHasHandler(t *testing.T) {
	t.Parallel()

	bus := eventbus.New()
	eventbus.Require[*eventA](bus)
	eventbus.Subscribe(bus, func(context.Context, *eventA) error { return nil })

	require.NoError(t, bus.Validate())
}

func TestValidateFailsWhenRequiredTypeHasNoHandler(t *testing.T) {
	t.Parallel()

	bus := eventbus.New()
	eventbus.Require[*eventA](bus)

	err := bus.Validate()
	require.ErrorIs(t, err, eventbus.ErrNoHandler)
}

func TestValidateIgnoresTypesThatWereNotRequired(t *testing.T) {
	t.Parallel()

	bus := eventbus.New()
	// *eventA has no handler but was never required → not an error.
	eventbus.Require[*eventB](bus)
	eventbus.Subscribe(bus, func(context.Context, *eventB) error { return nil })

	require.NoError(t, bus.Validate())
}

func TestValidateAdapterDoesNotSatisfyRequirement(t *testing.T) {
	t.Parallel()

	bus := eventbus.New()
	eventbus.Require[*eventA](bus)
	// Only an adapter is registered; a required event needs a business handler.
	eventbus.RegisterAdapter(bus, func(context.Context, *eventA) error { return nil })

	require.ErrorIs(t, bus.Validate(), eventbus.ErrNoHandler)
}

func TestValidateReportsEveryMissingType(t *testing.T) {
	t.Parallel()

	bus := eventbus.New()
	eventbus.Require[*eventA](bus)
	eventbus.Require[*eventB](bus)

	err := bus.Validate()
	require.ErrorIs(t, err, eventbus.ErrNoHandler)
	require.Contains(t, err.Error(), "eventbus_test.eventA")
	require.Contains(t, err.Error(), "eventbus_test.eventB")
}

func TestRequireAndValidateWorkThroughObservedBus(t *testing.T) {
	t.Parallel()

	observed := eventbus.NewObserved(eventbus.New(), metrics.Noop())
	eventbus.Require[*eventA](observed)

	require.ErrorIs(t, observed.Validate(), eventbus.ErrNoHandler)

	eventbus.Subscribe(observed, func(context.Context, *eventA) error { return nil })
	require.NoError(t, observed.Validate())
}
