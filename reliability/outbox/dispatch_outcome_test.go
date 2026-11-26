// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/runtime/concurrency"
	"github.com/altessa-s/go-atlas/reliability/outbox"
)

func TestDispatchCycleCancellationRestoresAttempt(t *testing.T) {
	t.Parallel()

	store := &batchStore{batch: []outbox.Event{{Id: "running", Key: "test"}}}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	out := outbox.New(store, func(ctx context.Context, _ outbox.Event) error {
		cancel()
		return fmt.Errorf("dispatch interrupted: %w", ctx.Err())
	}, outbox.WithRetryMaxAttempts(1))

	require.NoError(t, out.RunDispatchCycle(ctx))
	require.Len(t, store.updated, 1)
	require.Equal(t, outbox.StatusFailed, store.updated[0].Status)
	require.Zero(t, store.updated[0].Attempts)
}

func TestDispatchCyclePersistsMixedOutcomes(t *testing.T) {
	t.Parallel()

	// Every worker other than the one sending "sent" stays blocked. When
	// "running" starts, "sent" has returned and "unstarted" is still queued.
	batch := []outbox.Event{{Id: "sent", Key: "test"}}
	for i := range concurrency.DefaultLimitFunc() - 1 {
		batch = append(batch, outbox.Event{Id: fmt.Sprint(i), Key: "test"})
	}
	batch = append(batch, outbox.Event{Id: "running", Key: "test"},
		outbox.Event{Id: "unstarted", Key: "test", Attempts: 1, LockedOn: time.Now(), LockToken: "claim"})
	store := &batchStore{batch: batch}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	out := outbox.New(store, func(ctx context.Context, event outbox.Event) error {
		switch event.Id {
		case "sent":
			return nil
		case "running":
			cancel()
		}
		<-ctx.Done()
		return ctx.Err()
	}, outbox.WithRetryMaxAttempts(2))

	require.NoError(t, out.RunDispatchCycle(ctx))
	byID := make(map[string]outbox.Event, len(store.updated))
	for _, event := range store.updated {
		byID[event.Id] = event
	}
	require.Equal(t, len(batch), len(byID))
	require.Equal(t, outbox.StatusSent, byID["sent"].Status)
	require.EqualValues(t, 1, byID["sent"].Attempts)
	require.Equal(t, outbox.StatusFailed, byID["running"].Status)
	require.Zero(t, byID["running"].Attempts)
	require.Equal(t, outbox.StatusPending, byID["unstarted"].Status)
	require.EqualValues(t, 1, byID["unstarted"].Attempts)
	require.Zero(t, byID["unstarted"].LockedOn)
	require.Equal(t, "claim", byID["unstarted"].LockToken)
}

func TestDispatchCycleHandleTimeoutConsumesAttempts(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		store := &batchStore{batch: []outbox.Event{{Id: "hanging", Key: "test"}}}
		out := outbox.New(store, func(ctx context.Context, _ outbox.Event) error {
			<-ctx.Done()
			return ctx.Err()
		}, outbox.WithRetryMaxAttempts(2), outbox.WithHandleTimeout(time.Second))

		require.NoError(t, out.RunDispatchCycle(t.Context()))
		require.Len(t, store.updated, 1)
		require.Equal(t, outbox.StatusFailed, store.updated[0].Status)
		require.EqualValues(t, 1, store.updated[0].Attempts)

		store.batch = []outbox.Event{store.updated[0]}
		require.NoError(t, out.RunDispatchCycle(t.Context()))
		require.Len(t, store.updated, 2)
		require.Equal(t, outbox.StatusMaxAttemptReached, store.updated[1].Status)
		require.EqualValues(t, 2, store.updated[1].Attempts)
	})
}

func TestDispatchCycleParentDeadlineConsumesAttempts(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		store := &batchStore{batch: []outbox.Event{{Id: "hanging", Key: "test"}}}
		out := outbox.New(store, func(ctx context.Context, _ outbox.Event) error {
			<-ctx.Done()
			return ctx.Err()
		}, outbox.WithRetryMaxAttempts(2), outbox.WithHandleTimeout(20*time.Second))

		run := func() {
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			require.NoError(t, out.RunDispatchCycle(ctx))
		}
		run()
		require.Len(t, store.updated, 1)
		require.Equal(t, outbox.StatusFailed, store.updated[0].Status)
		require.EqualValues(t, 1, store.updated[0].Attempts)
		require.Positive(t, store.updated[0].RetryAfter)

		store.batch = []outbox.Event{store.updated[0]}
		time.Sleep(store.updated[0].RetryAfter)
		run()
		require.Len(t, store.updated, 2)
		require.Equal(t, outbox.StatusMaxAttemptReached, store.updated[1].Status)
		require.EqualValues(t, 2, store.updated[1].Attempts)
	})
}

func TestDispatchCycleBoundsHandlerDeadline(t *testing.T) {
	t.Parallel()

	const handleTimeout = 50 * time.Millisecond
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	store := &batchStore{batch: []outbox.Event{{Id: "deadline", Key: "test"}}}
	var deadline time.Time
	out := outbox.New(store, func(ctx context.Context, _ outbox.Event) error {
		deadline, _ = ctx.Deadline()
		return nil
	}, outbox.WithHandleTimeout(handleTimeout))

	require.NoError(t, out.RunDispatchCycle(ctx))
	require.False(t, deadline.IsZero())
	require.LessOrEqual(t, time.Until(deadline), handleTimeout)
}

func TestDispatchCycleRecoversHandlerPanic(t *testing.T) {
	t.Parallel()

	const child = "OUTBOX_PANIC_TEST_CHILD"
	const payload = "private-panic-payload"
	if os.Getenv(child) == "" {
		cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestDispatchCycleRecoversHandlerPanic$")
		cmd.Env = append(os.Environ(), child+"=1")
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", output)
		require.NotContains(t, string(output), payload)
		return
	}

	store := &batchStore{batch: []outbox.Event{{Id: "panic", Key: "test"}, {Id: "sent", Key: "test"}}}
	out := outbox.New(store, func(_ context.Context, event outbox.Event) error {
		if event.Id == "panic" {
			panic(payload)
		}
		return nil
	})
	require.NoError(t, out.RunDispatchCycle(t.Context()))
	require.Len(t, store.updated, 2)
	require.Equal(t, outbox.StatusFailed, store.updated[0].Status)
	require.EqualValues(t, 1, store.updated[0].Attempts)
	require.NotNil(t, store.updated[0].LastError)
	require.NotContains(t, *store.updated[0].LastError, payload)
	require.Equal(t, outbox.StatusSent, store.updated[1].Status)
}
