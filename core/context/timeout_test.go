// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package context_test

import (
	"context"
	"testing"
	"time"

	corecontext "github.com/altessa-s/go-atlas/core/context"
)

func TestApplyTimeout(t *testing.T) {
	t.Run("zero timeout returns original", func(t *testing.T) {
		ctx := t.Context()
		gotCtx, cancel := corecontext.ApplyTimeout(ctx, 0)
		defer cancel()
		if gotCtx != ctx {
			t.Error("expected original context")
		}
	})

	t.Run("context with deadline returns original", func(t *testing.T) {
		ctx, cancelOrig := context.WithTimeout(t.Context(), time.Second)
		defer cancelOrig()

		gotCtx, cancel := corecontext.ApplyTimeout(ctx, time.Minute)
		defer cancel()

		if gotCtx != ctx {
			t.Error("expected original context")
		}
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
		if !ok {
			t.Fatal("expected deadline to be set")
		}
		if !deadline.Equal(origDeadline) {
			t.Errorf("expected deadline %v, got %v", origDeadline, deadline)
		}
	})

	t.Run("context without deadline gets default", func(t *testing.T) {
		ctx, cancel := corecontext.WithDefault(t.Context(), time.Second)
		defer cancel()
		if _, ok := ctx.Deadline(); !ok {
			t.Error("expected deadline to be set")
		}
	})

	t.Run("nil context defaults to background", func(t *testing.T) {
		ctx, cancel := corecontext.WithDefault(nil, time.Millisecond) //nolint:staticcheck // intentionally passing nil
		defer cancel()
		if ctx.Err() == nil {
			<-ctx.Done()
		}
		if ctx.Err() != context.DeadlineExceeded {
			t.Errorf("expected DeadlineExceeded, got %v", ctx.Err())
		}
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
		if !ok {
			t.Fatal("expected deadline to be set")
		}
		if !deadline.Equal(origDeadline) {
			t.Errorf("expected deadline %v, got %v", origDeadline, deadline)
		}
	})

	t.Run("context with longer deadline gets capped", func(t *testing.T) {
		origCtx, origCancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer origCancel()
		origDeadline, _ := origCtx.Deadline()

		ctx, cancel := corecontext.WithMaxTimeout(origCtx, time.Second)
		defer cancel()
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("expected deadline to be set")
		}
		if !deadline.Before(origDeadline) {
			t.Errorf("expected capped deadline before %v, got %v", origDeadline, deadline)
		}
	})

	t.Run("zero max timeout returns original context", func(t *testing.T) {
		ctx, cancel := corecontext.WithMaxTimeout(t.Context(), 0)
		defer cancel()
		if _, ok := ctx.Deadline(); ok {
			t.Error("expected no deadline to be set")
		}
	})

	t.Run("nil context defaults to background", func(t *testing.T) {
		ctx, cancel := corecontext.WithMaxTimeout(nil, time.Minute) //nolint:staticcheck // intentionally passing nil
		defer cancel()
		if ctx == nil {
			t.Error("expected non-nil context")
		}
	})
}

func TestOrBackground(t *testing.T) {
	if corecontext.OrBackground(nil) == nil {
		t.Error("OrBackground(nil) returned nil")
	}
	ctx := t.Context()
	if corecontext.OrBackground(ctx) != ctx {
		t.Error("OrBackground(ctx) did not return ctx")
	}
}
