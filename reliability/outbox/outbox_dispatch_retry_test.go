// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// recordingStore captures the events handed to UpdateEvents so the tests can
// assert on the state the outbox decided to persist.
type recordingStore struct {
	fetch    []Event
	updated  []Event
	fetched  atomic.Int64
	unlocked atomic.Int64
}

func (s *recordingStore) FetchUnprocessedEvents(context.Context, uint32) ([]Event, error) {
	if s.fetched.Add(1) > 1 {
		return nil, nil // Only serve the batch once; later cycles find nothing.
	}
	return s.fetch, nil
}
func (s *recordingStore) DeleteProcessedEvents(context.Context, time.Duration) error { return nil }
func (s *recordingStore) UnlockStuckEvents(context.Context, time.Duration) error {
	s.unlocked.Add(1)
	return nil
}
func (s *recordingStore) SaveEvents(context.Context, ...Event) error { return nil }
func (s *recordingStore) UpdateEvents(_ context.Context, events ...Event) error {
	s.updated = append(s.updated, events...)
	return nil
}
func (s *recordingStore) ExpireEvents(context.Context) (int64, error) { return 0, nil }
func (s *recordingStore) Stats(context.Context) (Stats, error)        { return Stats{}, nil }

// poisonHandler returns a transient-looking error on every invocation
// and counts how many times it was called.
func poisonHandler(counter *atomic.Int64) Handler {
	return func(_ context.Context, _ Event) error {
		counter.Add(1)
		return errors.New("transient downstream failure")
	}
}

// TestDispatchEvent_SingleAttemptPerCycle pins the property that replaced the
// in-process retry loop: one cycle spends exactly one attempt. The old loop
// retried inside dispatchEvent while holding the event's store lock, which both
// multiplied the configured attempt budget (retryMaxAttempts squared) and let
// the lock expire mid-flight so a second worker could claim the same event.
func TestDispatchEvent_SingleAttemptPerCycle(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64
	ob := New(&recordingStore{}, poisonHandler(&calls), WithRetryMaxAttempts(5))

	require.Error(t, ob.dispatchEvent(t.Context(), Event{Id: "poison-1"}))
	require.Equal(t, int64(1), calls.Load(),
		"dispatchEvent must invoke the handler exactly once; retries are the store's job")
}

// TestHandleEvents_SchedulesExponentialBackoff verifies that a retryable
// failure leaves the event failed with a positive RetryAfter, and that the
// delay grows with the attempt count. A flat interval would keep hammering a
// dependency that is already failing.
func TestHandleEvents_SchedulesExponentialBackoff(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64
	store := &recordingStore{}
	ob := New(store, poisonHandler(&calls),
		WithRetryMaxAttempts(10),
		WithRetryBaseDelay(time.Second),
		WithRetryMaxDelay(time.Hour),
	)

	ob.handleEvents(t.Context(),
		Event{Id: "first-failure", Key: "k", Attempts: 0},
		Event{Id: "later-failure", Key: "k", Attempts: 5},
	)

	require.Len(t, store.updated, 2)

	byId := make(map[string]Event, len(store.updated))
	for _, e := range store.updated {
		byId[e.Id] = e
	}

	first := byId["first-failure"]
	require.Equal(t, StatusFailed, first.Status)
	require.Positive(t, first.RetryAfter, "a retryable failure must schedule a backoff")

	later := byId["later-failure"]
	require.Greater(t, later.RetryAfter, first.RetryAfter,
		"backoff must grow with the attempt count")
}

// TestHandleEvents_RejectsPermanentFailureImmediately covers the transient /
// permanent split the guide requires: an error the predicate calls permanent is
// dead-lettered on the spot instead of consuming the whole attempt budget.
func TestHandleEvents_RejectsPermanentFailureImmediately(t *testing.T) {
	t.Parallel()

	permanent := errors.New("malformed payload")
	var calls atomic.Int64
	store := &recordingStore{}

	ob := New(store, func(_ context.Context, _ Event) error {
		calls.Add(1)
		return permanent
	},
		WithRetryMaxAttempts(10),
		WithShouldRetry(func(err error) bool { return !errors.Is(err, permanent) }),
	)

	ob.handleEvents(t.Context(), Event{Id: "bad-json", Key: "k"})

	require.Equal(t, int64(1), calls.Load(), "a permanent failure must not be retried")
	require.Len(t, store.updated, 1)
	require.Equal(t, StatusRejected, store.updated[0].Status)
	require.Zero(t, store.updated[0].RetryAfter, "a dead-lettered event must not be rescheduled")
	require.NotNil(t, store.updated[0].LastError)
}

// TestHandleEvents_DeadLettersWhenBudgetExhausted asserts the terminal state at
// the end of the budget, so a poison message stops circulating instead of
// looping forever.
func TestHandleEvents_DeadLettersWhenBudgetExhausted(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64
	store := &recordingStore{}
	ob := New(store, poisonHandler(&calls), WithRetryMaxAttempts(3))

	// Attempts=2 -> nextAttempt() makes it 3, which equals the budget.
	ob.handleEvents(t.Context(), Event{Id: "poison-1", Key: "k", Attempts: 2})

	require.Len(t, store.updated, 1)
	require.Equal(t, StatusMaxAttemptReached, store.updated[0].Status)
	require.Zero(t, store.updated[0].RetryAfter)
}

// TestHandleEvents_ContextCancellationIsTransient guards against dead-lettering
// healthy events during an outage: our own cycle timing out says nothing about
// the event, so it must stay retryable even when a shouldRetry predicate is set.
func TestHandleEvents_ContextCancellationIsTransient(t *testing.T) {
	t.Parallel()

	store := &recordingStore{}
	ob := New(store, func(ctx context.Context, _ Event) error {
		<-ctx.Done()
		return ctx.Err()
	},
		WithRetryMaxAttempts(10),
		// Deliberately hostile: everything is permanent unless the outbox
		// special-cases context cancellation before consulting the predicate.
		WithShouldRetry(func(error) bool { return false }),
	)

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	ob.handleEvents(ctx, Event{Id: "slow", Key: "k"})

	require.Len(t, store.updated, 1)
	require.Equal(t, StatusFailed, store.updated[0].Status,
		"a cancelled cycle must leave the event retryable, not rejected")
}

// TestNew_RaisesLockTimeAboveHandleTimeout is the regression guard for the
// duplicate-dispatch window: with maxLockTime <= handleTimeout the unlock
// sweeper reclaims events that are still being published.
func TestNew_RaisesLockTimeAboveHandleTimeout(t *testing.T) {
	t.Parallel()

	ob := New(&recordingStore{}, noopHandler,
		WithHandleTimeout(20*time.Second),
		WithMaxLockTime(10*time.Second),
	)

	require.Greater(t, ob.maxLockTime, 20*time.Second,
		"a lock must outlive the dispatch cycle that holds it")
}

// TestNew_KeepsExplicitLockTimeAboveHandleTimeout confirms the clamp only fires
// when the relation is actually violated.
func TestNew_KeepsExplicitLockTimeAboveHandleTimeout(t *testing.T) {
	t.Parallel()

	ob := New(&recordingStore{}, noopHandler,
		WithHandleTimeout(5*time.Second),
		WithMaxLockTime(90*time.Second),
	)

	require.Equal(t, 90*time.Second, ob.maxLockTime)
}
