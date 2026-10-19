// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/retryqueue/storages/memory"
	"github.com/altessa-s/go-atlas/data/retryqueue/storages/storagetest"
)

func TestStorageContract(t *testing.T) {
	t.Parallel()
	storagetest.Run(t, func(testing.TB) storagetest.Storage { return memory.New[storagetest.Payload]() })
}

func TestLenCountsEveryState(t *testing.T) {
	t.Parallel()

	s := memory.New[storagetest.Payload]()
	require.Zero(t, s.Len())
	for _, key := range []string{"p", "e", "r"} {
		_, err := s.Mark(t.Context(), storagetest.Pending(key, storagetest.T0))
		require.NoError(t, err)
	}
	_, err := s.Expire(t.Context(), "e", 1, storagetest.T0, storagetest.T0)
	require.NoError(t, err)
	require.NoError(t, s.Resolve(t.Context(), "r", 1, storagetest.T0))
	require.Equal(t, 3, s.Len())
	require.NoError(t, s.Delete(t.Context(), "p"))
	require.Equal(t, 2, s.Len())
}

func TestTimesAreStoredInUTC(t *testing.T) {
	t.Parallel()

	s := memory.New[storagetest.Payload]()
	local := time.FixedZone("UTC+7", 7*60*60)
	in := storagetest.Pending("k", storagetest.T0.In(local))
	in.NextAttemptAt = storagetest.T0.In(local)
	got, err := s.Mark(t.Context(), in)
	require.NoError(t, err)
	require.Equal(t, time.UTC, got.Since.Location())
	require.Equal(t, time.UTC, got.NextAttemptAt.Location())

	require.NoError(t, s.Reschedule(t.Context(), "k", 1, storagetest.T0.Add(time.Hour).In(local)))
	due, err := s.ListDue(t.Context(), storagetest.T0.Add(time.Hour), 0)
	require.NoError(t, err)
	require.Len(t, due, 1)
	require.Equal(t, storagetest.T0.Add(time.Hour), due[0].NextAttemptAt)
}
