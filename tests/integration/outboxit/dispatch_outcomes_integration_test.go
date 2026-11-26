// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxit_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/reliability/outbox"
)

func TestDispatch_HandlerPanicPersistsFailure(t *testing.T) {
	t.Parallel()
	forEachBackend(t, func(t *testing.T, f *fixture) {
		const payload = "private-panic-payload"
		handler := func(_ context.Context, event outbox.Event) error {
			if event.Key == "panic" {
				panic(payload)
			}
			return nil
		}
		ob := outbox.New(f.store, handler)
		events := f.save(t, ob, outbox.Event{Key: "panic"}, outbox.Event{Key: "sent"})

		require.NoError(t, ob.RunDispatchCycle(t.Context()))
		failed := f.load(t, events[0].Id)
		require.Equal(t, string(outbox.StatusFailed), failed.Status)
		require.EqualValues(t, 1, failed.Attempts)
		require.NotNil(t, failed.LastError)
		require.Equal(t, outbox.ErrHandlerPanicked.Error(), *failed.LastError)
		require.Empty(t, failed.LockToken)

		sent := f.load(t, events[1].Id)
		require.Equal(t, string(outbox.StatusSent), sent.Status)
		require.EqualValues(t, 1, sent.Attempts)
		require.NotNil(t, sent.PublishedAt)
	})
}

func TestDispatch_CanceledCyclePreservesAttemptsAndConfirmedSend(t *testing.T) {
	t.Parallel()
	forEachBackend(t, func(t *testing.T, f *fixture) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		started := make(chan struct{})
		confirmed := make(chan struct{})
		done := make(chan error, 1)
		handler := func(ctx context.Context, event outbox.Event) error {
			if event.Key == "cancel" {
				close(started)
				<-ctx.Done()
				return ctx.Err()
			}
			close(confirmed)
			return nil
		}
		ob := outbox.New(f.store, handler, outbox.WithRetryMaxAttempts(1))
		events := f.save(t, ob, outbox.Event{Key: "cancel"}, outbox.Event{Key: "sent"})

		go func() { done <- ob.RunDispatchCycle(ctx) }()
		select {
		case <-started:
		case err := <-done:
			t.Fatalf("dispatch finished before the handler started: %v", err)
		}
		select {
		case <-confirmed:
		case err := <-done:
			t.Fatalf("dispatch finished before delivery was confirmed: %v", err)
		}
		cancel()
		require.NoError(t, <-done)

		failed := f.load(t, events[0].Id)
		require.Equal(t, string(outbox.StatusFailed), failed.Status)
		require.Zero(t, failed.Attempts)
		require.Empty(t, failed.LockToken)
		require.Nil(t, failed.LockedOn)
		require.Nil(t, failed.NextAttemptAt)

		sent := f.load(t, events[1].Id)
		require.Equal(t, string(outbox.StatusSent), sent.Status)
		require.EqualValues(t, 1, sent.Attempts)
		require.NotNil(t, sent.PublishedAt)

		recovered := f.newOutbox(t)
		require.NoError(t, recovered.RunDispatchCycle(t.Context()))
		require.Equal(t, 1, f.recorder.Count())
		require.Equal(t, string(outbox.StatusSent), f.load(t, events[0].Id).Status)
	})
}
