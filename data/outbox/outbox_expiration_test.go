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

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

type expirationStore struct {
	savedEvents  []Event
	expireCalls  atomic.Int64
	expireResult int64
	expireErr    error
}

func (s *expirationStore) FetchUnprocessedEvents(context.Context, uint32, time.Duration) ([]Event, error) {
	return nil, nil
}
func (s *expirationStore) DeleteProcessedEvents(context.Context, time.Duration) error { return nil }
func (s *expirationStore) UnlockStuckEvents(context.Context, time.Duration) error     { return nil }
func (s *expirationStore) SaveEvents(_ context.Context, events ...Event) error {
	s.savedEvents = append(s.savedEvents, events...)
	return nil
}
func (s *expirationStore) UpdateEvents(context.Context, ...Event) error { return nil }
func (s *expirationStore) ExpireEvents(_ context.Context) (int64, error) {
	s.expireCalls.Add(1)
	return s.expireResult, s.expireErr
}

func TestOutbox_Save_AppliesDefaultTTL(t *testing.T) {
	store := &expirationStore{}
	ttl := 5 * time.Minute

	ob := New(store, noopHandler, WithDefaultEventTTL(ttl))

	require.NoError(t, ob.Save(t.Context(), Event{Key: "test", Payload: []byte("data")}))

	require.Equal(t, 1, len(store.savedEvents))

	ev := store.savedEvents[0]
	require.False(t, ev.ExpiresAt.IsZero(), "ExpiresAt should be set by default TTL")

	expectedExpiry := ev.CreatedAt.Add(ttl)
	require.True(t, ev.ExpiresAt.Equal(expectedExpiry), "ExpiresAt = %v, want %v", ev.ExpiresAt, expectedExpiry)
}

func TestOutbox_Save_DoesNotOverrideExplicitExpiresAt(t *testing.T) {
	store := &expirationStore{}
	ttl := 5 * time.Minute
	customExpiry := time.Now().UTC().Add(1 * time.Hour)

	ob := New(store, noopHandler, WithDefaultEventTTL(ttl))

	require.NoError(t, ob.Save(t.Context(), Event{
		Key:       "test",
		Payload:   []byte("data"),
		ExpiresAt: customExpiry,
	}))

	ev := store.savedEvents[0]
	require.True(t, ev.ExpiresAt.Equal(customExpiry), "ExpiresAt = %v, want custom %v", ev.ExpiresAt, customExpiry)
}

func TestOutbox_Save_NoTTLWhenDisabled(t *testing.T) {
	store := &expirationStore{}

	ob := New(store, noopHandler) // No TTL option

	require.NoError(t, ob.Save(t.Context(), Event{Key: "test", Payload: []byte("data")}))

	ev := store.savedEvents[0]
	require.True(t, ev.ExpiresAt.IsZero(), "ExpiresAt should be zero when TTL is disabled, got %v", ev.ExpiresAt)
}

func TestWithDefaultEventTTL_IgnoresSubSecond(t *testing.T) {
	store := &expirationStore{}
	ob := New(store, noopHandler, WithDefaultEventTTL(500*time.Millisecond))

	require.NoError(t, ob.Save(t.Context(), Event{Key: "test", Payload: []byte("data")}))

	ev := store.savedEvents[0]
	require.True(t, ev.ExpiresAt.IsZero(), "ExpiresAt should be zero when TTL < 1s")
}

func TestOutbox_RunExpireCycle(t *testing.T) {
	store := &expirationStore{expireResult: 3}
	ob := New(store, noopHandler)

	require.NoError(t, ob.RunExpireCycle(t.Context()))
	require.Equal(t, int64(1), store.expireCalls.Load())
}

func TestOutbox_RunExpireCycle_SchedulerManaged(t *testing.T) {
	store := &expirationStore{}
	ob := New(store, noopHandler)

	// Register with scheduler to make it managed
	_ = ob.RegisterExpireSchedulerFunc()

	err := ob.RunExpireCycle(t.Context())
	require.ErrorIs(t, err, corescheduler.ErrSchedulerManaged)
}

func TestOutbox_RunExpireCycle_PropagatesStoreError(t *testing.T) {
	storeErr := errors.New("mongo unavailable")
	store := &expirationStore{expireErr: storeErr}
	ob := New(store, noopHandler)

	err := ob.RunExpireCycle(t.Context())
	require.Error(t, err, "RunExpireCycle should return error")
	require.ErrorIs(t, err, storeErr)
}

func TestWithDefaultEventTTL_AcceptsExactOneSecond(t *testing.T) {
	store := &expirationStore{}
	ob := New(store, noopHandler, WithDefaultEventTTL(time.Second))

	require.NoError(t, ob.Save(t.Context(), Event{Key: "test", Payload: []byte("data")}))

	ev := store.savedEvents[0]
	require.False(t, ev.ExpiresAt.IsZero(), "ExpiresAt should be set when TTL = 1s")
}

func TestWithDefaultEventTTL_IgnoresZero(t *testing.T) {
	store := &expirationStore{}
	ob := New(store, noopHandler, WithDefaultEventTTL(0))

	require.NoError(t, ob.Save(t.Context(), Event{Key: "test", Payload: []byte("data")}))

	ev := store.savedEvents[0]
	require.True(t, ev.ExpiresAt.IsZero(), "ExpiresAt should be zero when TTL = 0, got %v", ev.ExpiresAt)
}
