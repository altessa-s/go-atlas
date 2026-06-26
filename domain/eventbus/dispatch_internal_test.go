// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package eventbus

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// White-box tests for dispatch's observability summary (used by observedBus).
// They need the unexported dispatch/dispatchResult, so they live in package
// eventbus rather than eventbus_test.

type reqEvent struct{}

type plainEvent struct{}

func TestDispatchReportsRequiredWhenNoHandler(t *testing.T) {
	t.Parallel()

	eb := New()
	eb.RequireHandler(&reqEvent{})

	res, err := eb.dispatch(t.Context(), &reqEvent{})
	require.NoError(t, err)
	require.Equal(t, 0, res.Handlers)
	require.True(t, res.Required, "a required type with no handler must be flagged")
}

func TestDispatchCountsHandlersAndClearsRequired(t *testing.T) {
	t.Parallel()

	eb := New()
	eb.RequireHandler(&reqEvent{})
	Subscribe(eb, func(context.Context, *reqEvent) error { return nil })

	res, err := eb.dispatch(t.Context(), &reqEvent{})
	require.NoError(t, err)
	require.Equal(t, 1, res.Handlers)
	require.False(t, res.Required, "a handler ran, so the no-handler flag must not be raised")
}

func TestDispatchNotRequiredWhenNoHandler(t *testing.T) {
	t.Parallel()

	eb := New()

	res, err := eb.dispatch(t.Context(), &plainEvent{})
	require.NoError(t, err)
	require.Equal(t, 0, res.Handlers)
	require.False(t, res.Required, "a non-required type must not be flagged")
}
