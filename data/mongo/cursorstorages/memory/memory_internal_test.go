// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/mongo"
)

// TestStorage_ExpiredCleanupKeepsConcurrentRefresh forces the interleaving in
// which Load observes an expired entry, a Store replaces it, and only then does
// Load's cleanup run: the fresh entry must survive.
func TestStorage_ExpiredCleanupKeepsConcurrentRefresh(t *testing.T) {
	t.Parallel()

	s := New(time.Hour)
	defer s.Close()

	const key = "k"
	expired := &entry{metadata: &mongo.CursorMetadata{}, expiresAt: time.Now().Add(-time.Minute)}
	s.store[key] = expired

	fresh := &mongo.CursorMetadata{}
	require.NoError(t, s.Store(t.Context(), key, fresh))

	s.deleteIfSame(key, expired)

	got, err := s.Load(t.Context(), key)
	require.NoError(t, err, "the refreshed entry must not be removed by stale cleanup")
	require.Same(t, fresh, got)
}

func TestStorage_LoadRemovesUntouchedExpiredEntry(t *testing.T) {
	t.Parallel()

	s := New(time.Hour)
	defer s.Close()

	const key = "k"
	s.store[key] = &entry{metadata: &mongo.CursorMetadata{}, expiresAt: time.Now().Add(-time.Minute)}

	_, err := s.Load(t.Context(), key)
	require.ErrorIs(t, err, mongo.ErrCursorNotFound)

	s.mu.RLock()
	_, exists := s.store[key]
	s.mu.RUnlock()
	require.False(t, exists, "an expired entry nobody replaced must be removed")
}
