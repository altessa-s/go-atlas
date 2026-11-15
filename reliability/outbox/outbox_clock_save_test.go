// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// clockedStore is a store that stamps new events from its own clock.
type clockedStore struct {
	recordingStore
	ttl   time.Duration
	saved []Event
	plain bool // SaveEvents was called instead of SaveEventsWithTTL
}

func (s *clockedStore) SaveEvents(context.Context, ...Event) error {
	s.plain = true
	return nil
}

func (s *clockedStore) SaveEventsWithTTL(_ context.Context, ttl time.Duration, events ...Event) error {
	s.ttl, s.saved = ttl, append(s.saved, events...)
	return nil
}

// plainStore records what a store without a clock of its own is handed.
type plainStore struct {
	recordingStore
	saved []Event
}

func (s *plainStore) SaveEvents(_ context.Context, events ...Event) error {
	s.saved = append(s.saved, events...)
	return nil
}

// A store that stamps from its backend clock gets the instants the caller set
// — and nothing else — plus the default TTL as a duration: a creation time or
// expiry computed here would be a second clock source next to the one every
// later predicate uses.
func TestSave_BackendClockStoreStampsItsOwnInstants(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	store := &clockedStore{}
	ob := New(store, noopHandler, WithDefaultEventTTL(time.Hour))

	require.NoError(t, ob.Save(t.Context(),
		Event{Key: "undated"},
		Event{Key: "dated", CreatedAt: created, ExpiresAt: created.Add(time.Minute)},
	))

	require.False(t, store.plain)
	require.Equal(t, time.Hour, store.ttl)
	require.Len(t, store.saved, 2)
	require.NotEmpty(t, store.saved[0].Id)
	require.Equal(t, StatusPending, store.saved[0].Status)
	require.Zero(t, store.saved[0].CreatedAt, "the store stamps the creation time")
	require.Zero(t, store.saved[0].ExpiresAt, "the store derives the expiry from the TTL")
	require.Equal(t, created, store.saved[1].CreatedAt)
	require.Equal(t, created.Add(time.Minute), store.saved[1].ExpiresAt)
}

// Any other store keeps getting both instants from the local clock.
func TestSave_PlainStoreIsStampedLocally(t *testing.T) {
	t.Parallel()

	store := &plainStore{}
	ob := New(store, noopHandler, WithDefaultEventTTL(time.Hour))

	before := time.Now().UTC()
	require.NoError(t, ob.Save(t.Context(), Event{Key: "undated"}))

	require.Len(t, store.saved, 1)
	require.WithinDuration(t, before, store.saved[0].CreatedAt, time.Minute)
	require.Equal(t, store.saved[0].CreatedAt.Add(time.Hour), store.saved[0].ExpiresAt)
}
