// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/keyset"
	"github.com/altessa-s/go-atlas/service/scheduler"
)

// Without a page-token codec the scheduler schedules as usual and only the
// paginated lists fail, with ErrPageTokensNotConfigured.
func TestPaginatedListsNeedPageTokens(t *testing.T) {
	t.Parallel()

	s := scheduler.New(mustNewMemory(t, 100), scheduler.WithTickInterval(time.Hour))
	registerDummyTasks(t, s, 2)
	_, err := s.TasksPaginated(t.Context(), scheduler.PageRequest{}, "")
	require.ErrorIs(t, err, scheduler.ErrPageTokensNotConfigured)
	_, err = s.HistoryPaginated(t.Context(), generatePaddedID(0), scheduler.PageRequest{}, "")
	require.ErrorIs(t, err, scheduler.ErrPageTokensNotConfigured)
}

// Two independently constructed schedulers sharing a storage and a codec —
// two replicas, or one replica before and after a restart — continue each
// other's cursors.
func TestPaginatedListsAcrossSchedulers(t *testing.T) {
	t.Parallel()

	codec, err := keyset.New(bytes.Repeat([]byte{9}, keyset.MinKeyLength))
	require.NoError(t, err)
	storage := mustNewMemory(t, 100)
	a := scheduler.New(storage, scheduler.WithTickInterval(time.Hour), scheduler.WithPageTokens(codec))
	registerDummyTasks(t, a, 3)
	b := scheduler.New(storage, scheduler.WithTickInterval(time.Hour), scheduler.WithPageTokens(codec))

	first, err := a.TasksPaginated(t.Context(), scheduler.PageRequest{Limit: 2}, "")
	require.NoError(t, err)
	require.Len(t, first.Items, 2)
	require.NotNil(t, first.NextCursor)
	second, err := b.TasksPaginated(t.Context(), scheduler.PageRequest{Limit: 2, Cursor: *first.NextCursor}, "")
	require.NoError(t, err)
	require.Len(t, second.Items, 1)
	require.Equal(t, generatePaddedID(2), second.Items[0].ID)
	require.Nil(t, second.NextCursor)
}

// WithEphemeralPageTokens pages within one instance; another instance does
// not accept its cursors.
func TestEphemeralPageTokens(t *testing.T) {
	t.Parallel()

	storage := mustNewMemory(t, 100)
	s := scheduler.New(storage, scheduler.WithTickInterval(time.Hour), scheduler.WithEphemeralPageTokens())
	registerDummyTasks(t, s, 3)
	first, err := s.TasksPaginated(t.Context(), scheduler.PageRequest{Limit: 2}, "")
	require.NoError(t, err)
	require.NotNil(t, first.NextCursor)
	next, err := s.TasksPaginated(t.Context(), scheduler.PageRequest{Limit: 2, Cursor: *first.NextCursor}, "")
	require.NoError(t, err)
	require.Len(t, next.Items, 1)

	other := scheduler.New(storage, scheduler.WithTickInterval(time.Hour), scheduler.WithEphemeralPageTokens())
	_, err = other.TasksPaginated(t.Context(), scheduler.PageRequest{Limit: 2, Cursor: *first.NextCursor}, "")
	require.ErrorIs(t, err, scheduler.ErrInvalidCursor)
}
