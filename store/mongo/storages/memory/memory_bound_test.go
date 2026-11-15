// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/store/mongo"
	"github.com/altessa-s/go-atlas/store/mongo/internal/testhelpers"
	"github.com/altessa-s/go-atlas/store/mongo/storages/memory"
)

func TestStorage_Store_EvictsOldestAtMaxEntries(t *testing.T) {
	t.Parallel()

	const maxEntries = 3
	s := memory.New(time.Hour, memory.WithMaxEntries(maxEntries))
	ctx := t.Context()

	for i := range 10 {
		require.NoError(t, s.Store(ctx, fmt.Sprintf("key-%d", i), testhelpers.SampleCursorMetadata()))
		require.LessOrEqual(t, s.Len(), maxEntries)
	}

	_, err := s.Load(ctx, "key-6")
	require.ErrorIs(t, err, mongo.ErrCursorNotFound, "the oldest cursors are evicted")
	for i := 7; i < 10; i++ {
		_, err = s.Load(ctx, fmt.Sprintf("key-%d", i))
		require.NoError(t, err, "the newest cursors survive")
	}
}

func TestStorage_Store_DropsExpiredWithoutCleanup(t *testing.T) {
	t.Parallel()

	s := memory.New(time.Nanosecond)
	ctx := t.Context()

	for i := range 100 {
		require.NoError(t, s.Store(ctx, fmt.Sprintf("key-%d", i), testhelpers.SampleCursorMetadata()))
	}
	require.Eventually(t, func() bool {
		require.NoError(t, s.Store(ctx, "last", testhelpers.SampleCursorMetadata()))
		return s.Len() == 1
	}, time.Second, time.Millisecond, "Store removes expired cursors on its own")
}

func TestStorage_Store_ReplacedAndDeletedKeysStayBounded(t *testing.T) {
	t.Parallel()

	const maxEntries = 4
	s := memory.New(time.Hour, memory.WithMaxEntries(maxEntries))
	ctx := t.Context()

	for i := range 100 {
		require.NoError(t, s.Store(ctx, "same", testhelpers.SampleCursorMetadata()))
		key := fmt.Sprintf("key-%d", i)
		require.NoError(t, s.Store(ctx, key, testhelpers.SampleCursorMetadata()))
		require.NoError(t, s.Delete(ctx, key))
		require.LessOrEqual(t, s.Len(), maxEntries)
	}

	_, err := s.Load(ctx, "same")
	require.NoError(t, err, "a key stored again is not evicted through its stale position")
}

func TestStorage_Store_ConcurrentStaysBounded(t *testing.T) {
	t.Parallel()

	const maxEntries = 16
	s := memory.New(time.Hour, memory.WithMaxEntries(maxEntries))
	ctx := t.Context()

	var wg sync.WaitGroup
	for w := range 8 {
		wg.Go(func() {
			for i := range 200 {
				key := fmt.Sprintf("w%d-%d", w, i)
				_ = s.Store(ctx, key, testhelpers.SampleCursorMetadata())
				_, _ = s.Load(ctx, key)
				s.RunCleanup()
			}
		})
	}
	wg.Wait()

	require.LessOrEqual(t, s.Len(), maxEntries)
}
