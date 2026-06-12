// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package context_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	corecontext "github.com/altessa-s/go-atlas/core/context"
)

func TestApplyTimeout(t *testing.T) {
	t.Run("zero timeout returns original", func(t *testing.T) {
		ctx := t.Context()
		gotCtx, cancel := corecontext.ApplyTimeout(ctx, 0)
		defer cancel()
		require.Equal(t, ctx, gotCtx, "expected original context")
	})

	t.Run("context with deadline returns original", func(t *testing.T) {
		ctx, cancelOrig := context.WithTimeout(t.Context(), time.Second)
		defer cancelOrig()

		gotCtx, cancel := corecontext.ApplyTimeout(ctx, time.Minute)
		defer cancel()

		require.Equal(t, ctx, gotCtx, "expected original context")
	})

	t.Run("applies timeout", func(t *testing.T) {
		gotCtx, cancel := corecontext.ApplyTimeout(t.Context(), time.Millisecond)
		defer cancel()

		select {
		case <-gotCtx.Done():
		case <-time.After(time.Second):
			t.Error("timeout not applied")
		}
	})
}

func TestWithDefault(t *testing.T) {
	t.Run("context with deadline remains unchanged", func(t *testing.T) {
		origCtx, origCancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer origCancel()
		origDeadline, _ := origCtx.Deadline()

		ctx, cancel := corecontext.WithDefault(origCtx, time.Second)
		defer cancel()
		deadline, ok := ctx.Deadline()
		require.True(t, ok, "expected deadline to be set")
		require.True(t, deadline.Equal(origDeadline), "expected deadline %v, got %v", origDeadline, deadline)
	})

	t.Run("context without deadline gets default", func(t *testing.T) {
		ctx, cancel := corecontext.WithDefault(t.Context(), time.Second)
		defer cancel()
		_, ok := ctx.Deadline()
		require.True(t, ok, "expected deadline to be set")
	})

	t.Run("zero timeout is a no-op", func(t *testing.T) {
		origCtx := t.Context()
		ctx, cancel := corecontext.WithDefault(origCtx, 0)
		defer cancel()
		require.Equal(t, origCtx, ctx, "expected original context")
		_, ok := ctx.Deadline()
		require.False(t, ok, "expected no deadline to be set")
		require.NoError(t, ctx.Err(), "expected context not to be canceled")
	})

	t.Run("nil context defaults to background", func(t *testing.T) {
		ctx, cancel := corecontext.WithDefault(nil, time.Millisecond) //nolint:staticcheck // intentionally passing nil
		defer cancel()
		if ctx.Err() == nil {
			<-ctx.Done()
		}
		require.Equal(t, context.DeadlineExceeded, ctx.Err())
	})
}

func TestWithMaxTimeout(t *testing.T) {
	t.Run("context with shorter deadline remains unchanged", func(t *testing.T) {
		origCtx, origCancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
		defer origCancel()
		origDeadline, _ := origCtx.Deadline()

		ctx, cancel := corecontext.WithMaxTimeout(origCtx, time.Second)
		defer cancel()
		deadline, ok := ctx.Deadline()
		require.True(t, ok, "expected deadline to be set")
		require.True(t, deadline.Equal(origDeadline), "expected deadline %v, got %v", origDeadline, deadline)
	})

	t.Run("context with longer deadline gets capped", func(t *testing.T) {
		origCtx, origCancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer origCancel()
		origDeadline, _ := origCtx.Deadline()

		ctx, cancel := corecontext.WithMaxTimeout(origCtx, time.Second)
		defer cancel()
		deadline, ok := ctx.Deadline()
		require.True(t, ok, "expected deadline to be set")
		require.True(t, deadline.Before(origDeadline), "expected capped deadline before %v, got %v", origDeadline, deadline)
	})

	t.Run("zero max timeout returns original context", func(t *testing.T) {
		ctx, cancel := corecontext.WithMaxTimeout(t.Context(), 0)
		defer cancel()
		_, ok := ctx.Deadline()
		require.False(t, ok, "expected no deadline to be set")
	})

	t.Run("nil context defaults to background", func(t *testing.T) {
		ctx, cancel := corecontext.WithMaxTimeout(nil, time.Minute) //nolint:staticcheck // intentionally passing nil
		defer cancel()
		require.NotNil(t, ctx, "expected non-nil context")
	})
}

func TestOrBackground(t *testing.T) {
	require.NotNil(t, corecontext.OrBackground(nil), "OrBackground(nil) returned nil")
	ctx := t.Context()
	require.Equal(t, ctx, corecontext.OrBackground(ctx), "OrBackground(ctx) did not return ctx")
}
