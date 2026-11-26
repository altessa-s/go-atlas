// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxit_test

import (
	"context"
	"testing"
	"time"

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
		// A single worker must confirm the send before entering the blocking handler.
		now := time.Now()
		events := f.save(t, ob,
			outbox.Event{Key: "sent", CreatedAt: now.Add(-time.Second)},
			outbox.Event{Key: "cancel", CreatedAt: now})

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

		failed := f.load(t, events[1].Id)
		require.Equal(t, string(outbox.StatusFailed), failed.Status)
		require.Zero(t, failed.Attempts)
		require.Empty(t, failed.LockToken)
		require.Nil(t, failed.LockedOn)
		require.Nil(t, failed.NextAttemptAt)

		sent := f.load(t, events[0].Id)
		require.Equal(t, string(outbox.StatusSent), sent.Status)
		require.EqualValues(t, 1, sent.Attempts)
		require.NotNil(t, sent.PublishedAt)

		recovered := f.newOutbox(t)
		require.NoError(t, recovered.RunDispatchCycle(t.Context()))
		require.Equal(t, 1, f.recorder.Count())
		require.Equal(t, string(outbox.StatusSent), f.load(t, events[1].Id).Status)
	})
}

func TestDispatch_NeverStartedClaimsKeepAttemptHistory(t *testing.T) {
	t.Parallel()
	forEachBackend(t, func(t *testing.T, f *fixture) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		ob := outbox.New(f.store, func(context.Context, outbox.Event) error {
			panic("handler started after the cycle was canceled")
		}, outbox.WithCompaction(), outbox.WithCompactionFilter(func(string) bool {
			cancel()
			return true
		}))
		retried := f.save(t, ob, outbox.Event{Key: "retried"})[0]
		claims, err := f.store.FetchUnprocessedEvents(t.Context(), 1)
		require.NoError(t, err)
		require.Len(t, claims, 1)
		claims[0].Status = outbox.StatusFailed
		claims[0].Attempts = 1
		claims[0].LockedOn = time.Time{}
		require.NoError(t, f.store.UpdateEvents(t.Context(), claims...))
		before := f.load(t, retried.Id)
		require.NotNil(t, before.LastAttemptOn)

		fresh := f.save(t, ob, outbox.Event{Key: "fresh"})[0]
		require.NoError(t, ob.RunDispatchCycle(ctx))

		freshRow := f.load(t, fresh.Id)
		require.Zero(t, freshRow.Attempts)
		require.Nil(t, freshRow.LastAttemptOn)
		retriedRow := f.load(t, retried.Id)
		require.Equal(t, before.Attempts, retriedRow.Attempts)
		require.Equal(t, before.LastAttemptOn, retriedRow.LastAttemptOn)
		for _, row := range []storedEvent{freshRow, retriedRow} {
			require.Equal(t, string(outbox.StatusPending), row.Status)
			require.Empty(t, row.LockToken)
			require.Nil(t, row.LockedOn)
		}
	})
}
