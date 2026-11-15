// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/store/mongo"
	"github.com/altessa-s/go-atlas/store/mongo/internal/testhelpers"
	"github.com/altessa-s/go-atlas/store/mongo/storages/memory"
)

// subjectMetadata returns cursor metadata bound to subject.
func subjectMetadata(subject string) *mongo.CursorMetadata {
	m := testhelpers.SampleCursorMetadata()
	m.Subject = subject
	return m
}

// requireStored fails unless every key resolves.
func requireStored(t testing.TB, s *memory.Storage, keys ...string) {
	t.Helper()
	for _, key := range keys {
		_, err := s.Load(t.Context(), key)
		require.NoError(t, err, key)
	}
}

// requireEvicted fails unless every key is gone.
func requireEvicted(t testing.TB, s *memory.Storage, keys ...string) {
	t.Helper()
	for _, key := range keys {
		_, err := s.Load(t.Context(), key)
		require.ErrorIs(t, err, mongo.ErrCursorNotFound, key)
	}
}

// TestStorage_Store_CapCountsLiveEntriesOnly pins that maxEntries bounds the
// cursors that are stored, not the ones that ever were: keys deleted or stored
// again leave nothing behind that would push a live cursor out below the cap.
func TestStorage_Store_CapCountsLiveEntriesOnly(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		churn func(t *testing.T, s *memory.Storage, i int)
	}{
		{
			name: "deleted keys",
			churn: func(t *testing.T, s *memory.Storage, i int) {
				key := fmt.Sprintf("tmp-%d", i)
				require.NoError(t, s.Store(t.Context(), key, testhelpers.SampleCursorMetadata()))
				require.NoError(t, s.Delete(t.Context(), key))
			},
		},
		{
			name: "a key stored again",
			churn: func(t *testing.T, s *memory.Storage, _ int) {
				require.NoError(t, s.Store(t.Context(), "same", testhelpers.SampleCursorMetadata()))
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			const maxEntries = 4
			s := memory.New(time.Hour, memory.WithMaxEntries(maxEntries))
			live := []string{"live-0", "live-1", "live-2"}
			for _, key := range live {
				require.NoError(t, s.Store(t.Context(), key, testhelpers.SampleCursorMetadata()))
			}

			for i := range 100 {
				tc.churn(t, s, i)
				require.LessOrEqual(t, s.Len(), maxEntries)
			}

			requireStored(t, s, live...)
		})
	}
}

// TestStorage_Store_PerSubjectCap pins that a subject over its cap evicts its
// own oldest cursors and leaves the cursors of the other subjects alone.
func TestStorage_Store_PerSubjectCap(t *testing.T) {
	t.Parallel()

	const perSubject = 2
	s := memory.New(time.Hour, memory.WithMaxEntries(100), memory.WithMaxEntriesPerSubject(perSubject))
	ctx := t.Context()

	require.NoError(t, s.Store(ctx, "b-0", subjectMetadata("tenant-B")))
	require.NoError(t, s.Store(ctx, "b-1", subjectMetadata("tenant-B")))
	for i := range 50 {
		require.NoError(t, s.Store(ctx, fmt.Sprintf("a-%d", i), subjectMetadata("tenant-A")))
	}

	requireStored(t, s, "b-0", "b-1", "a-48", "a-49")
	requireEvicted(t, s, "a-0", "a-47")
	require.Equal(t, 2*perSubject, s.Len())
}

// TestStorage_Store_PerSubjectCapCountsLiveEntriesOnly pins that a deleted
// cursor frees its place under the subject's cap.
func TestStorage_Store_PerSubjectCapCountsLiveEntriesOnly(t *testing.T) {
	t.Parallel()

	s := memory.New(time.Hour, memory.WithMaxEntriesPerSubject(2))
	ctx := t.Context()

	require.NoError(t, s.Store(ctx, "a-0", subjectMetadata("tenant-A")))
	for i := range 20 {
		key := fmt.Sprintf("tmp-%d", i)
		require.NoError(t, s.Store(ctx, key, subjectMetadata("tenant-A")))
		require.NoError(t, s.Delete(ctx, key))
	}
	require.NoError(t, s.Store(ctx, "a-1", subjectMetadata("tenant-A")))

	requireStored(t, s, "a-0", "a-1")
}

// TestStorage_Store_PerSubjectCapSkipsUnboundCursors pins the documented
// limit of the per-subject cap: cursors without a subject share the global
// cap only.
func TestStorage_Store_PerSubjectCapSkipsUnboundCursors(t *testing.T) {
	t.Parallel()

	s := memory.New(time.Hour, memory.WithMaxEntriesPerSubject(2))
	keys := []string{"u-0", "u-1", "u-2", "u-3", "u-4"}
	for _, key := range keys {
		require.NoError(t, s.Store(t.Context(), key, testhelpers.SampleCursorMetadata()))
	}

	requireStored(t, s, keys...)
}

// TestStorage_Store_PerSubjectCapDisabled pins that 0 turns the per-subject
// cap off and a negative value keeps the configured one.
func TestStorage_Store_PerSubjectCapDisabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts []memory.Option
		want int
	}{
		{name: "zero disables", opts: []memory.Option{memory.WithMaxEntriesPerSubject(0)}, want: 5},
		{
			name: "negative is ignored",
			opts: []memory.Option{memory.WithMaxEntriesPerSubject(2), memory.WithMaxEntriesPerSubject(-1)},
			want: 2,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := memory.New(time.Hour, tc.opts...)
			for i := range 5 {
				require.NoError(t, s.Store(t.Context(), fmt.Sprintf("a-%d", i), subjectMetadata("tenant-A")))
			}
			require.Equal(t, tc.want, s.Len())
		})
	}
}

// TestStorage_Store_SubjectAtCapKeepsGlobalOrder pins that the two caps
// compose: once the storage is full, the globally oldest cursor goes, whoever
// owns it.
func TestStorage_Store_SubjectAtCapKeepsGlobalOrder(t *testing.T) {
	t.Parallel()

	s := memory.New(time.Hour, memory.WithMaxEntries(3), memory.WithMaxEntriesPerSubject(2))
	ctx := t.Context()

	require.NoError(t, s.Store(ctx, "a-0", subjectMetadata("tenant-A")))
	require.NoError(t, s.Store(ctx, "b-0", subjectMetadata("tenant-B")))
	require.NoError(t, s.Store(ctx, "c-0", subjectMetadata("tenant-C")))
	require.NoError(t, s.Store(ctx, "b-1", subjectMetadata("tenant-B")))

	requireEvicted(t, s, "a-0")
	requireStored(t, s, "b-0", "c-0", "b-1")
}
