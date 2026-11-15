// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/store/mongo"
)

// requireOrdersMatchStore fails unless the eviction orders hold exactly the
// live entries: one position per stored cursor, and no list for a subject
// without cursors.
func requireOrdersMatchStore(t testing.TB, s *Storage) {
	t.Helper()
	s.mu.RLock()
	defer s.mu.RUnlock()

	require.Equal(t, len(s.store), s.order.Len(), "the eviction order holds stale positions")
	bound := 0
	for subject, entries := range s.subjects {
		require.Positive(t, entries.Len(), "subject %q kept an empty list", subject)
		bound += entries.Len()
	}
	require.LessOrEqual(t, bound, len(s.store))
}

// TestStorage_OrderHoldsLiveEntriesOnly pins that removing a cursor, by any
// path, removes its position in the eviction orders too. A long-lived cursor
// at the head keeps Store from reaching the positions behind it, so they must
// not be left there to accumulate.
func TestStorage_OrderHoldsLiveEntriesOnly(t *testing.T) {
	t.Parallel()

	s := New(time.Hour, WithMaxEntriesPerSubject(8))
	ctx := t.Context()
	require.NoError(t, s.Store(ctx, "head", &mongo.CursorMetadata{Subject: "tenant-0"}))

	for i := range 200 {
		subject := fmt.Sprintf("tenant-%d", i%5)
		key := fmt.Sprintf("key-%d", i)
		require.NoError(t, s.Store(ctx, key, &mongo.CursorMetadata{Subject: subject}))
		require.NoError(t, s.Store(ctx, "same", &mongo.CursorMetadata{Subject: subject}))
		switch i % 3 {
		case 0:
			require.NoError(t, s.Delete(ctx, key))
		case 1:
			storeExpired(s, "expired")
			_, err := s.Load(ctx, "expired")
			require.ErrorIs(t, err, mongo.ErrCursorNotFound)
		}
		requireOrdersMatchStore(t, s)
	}

	require.NoError(t, s.Delete(ctx, "head"))
	require.NoError(t, s.Delete(ctx, "same"))
	for i := range 200 {
		require.NoError(t, s.Delete(ctx, fmt.Sprintf("key-%d", i)))
	}
	requireOrdersMatchStore(t, s)
	require.Zero(t, s.Len())
	require.Empty(t, s.subjects)
}

// TestStorage_RunCleanupDropsExpiredFromOrders pins that the cleanup cycle
// removes expired cursors together with their positions.
func TestStorage_RunCleanupDropsExpiredFromOrders(t *testing.T) {
	t.Parallel()

	s := New(time.Hour)
	for i := range 10 {
		e := &entry{
			key:       fmt.Sprintf("expired-%d", i),
			metadata:  &mongo.CursorMetadata{},
			subject:   "tenant-A",
			expiresAt: time.Now().Add(-time.Minute),
		}
		s.mu.Lock()
		s.insertLocked(e)
		s.mu.Unlock()
	}
	require.Equal(t, 10, s.Len())

	s.RunCleanup()

	require.Zero(t, s.Len())
	requireOrdersMatchStore(t, s)
	require.Empty(t, s.subjects)
}
