// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/store/mongo/storages/memory"
)

// TestStorage_Store_DefaultSubjectCapFollowsMaxEntries pins that the default
// per-subject cap is derived from the global one: with a lowered WithMaxEntries
// and nothing else, a subject paging in a loop still cannot evict the live
// cursors of another subject.
func TestStorage_Store_DefaultSubjectCapFollowsMaxEntries(t *testing.T) {
	t.Parallel()

	const maxEntries = 1000
	s := memory.New(time.Hour, memory.WithMaxEntries(maxEntries))
	ctx := t.Context()

	victim := make([]string, 0, 50)
	for i := range 50 {
		key := fmt.Sprintf("b-%d", i)
		victim = append(victim, key)
		require.NoError(t, s.Store(ctx, key, subjectMetadata("tenant-B")))
	}
	for i := range 5 * maxEntries {
		require.NoError(t, s.Store(ctx, fmt.Sprintf("a-%d", i), subjectMetadata("tenant-A")))
	}

	requireStored(t, s, victim...)
	require.Equal(t, len(victim)+maxEntries/10, s.Len(), "the noisy subject is held to a tenth of the global cap")
}

// TestStorage_Store_SubjectCapDerivation pins the effective per-subject cap
// for each way of configuring it, observed as the number of cursors one
// subject keeps.
func TestStorage_Store_SubjectCapDerivation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		opts   []memory.Option
		stores int
		want   int
	}{
		{name: "tenth of the global cap", opts: []memory.Option{memory.WithMaxEntries(50)}, stores: 20, want: 5},
		{name: "at least one", opts: []memory.Option{memory.WithMaxEntries(3)}, stores: 20, want: 1},
		{
			name:   "bounded by the default",
			opts:   []memory.Option{memory.WithMaxEntries(10*memory.DefaultMaxEntriesPerSubject + 50)},
			stores: memory.DefaultMaxEntriesPerSubject + 5,
			want:   memory.DefaultMaxEntriesPerSubject,
		},
		{
			name:   "explicit value is kept",
			opts:   []memory.Option{memory.WithMaxEntriesPerSubject(7), memory.WithMaxEntries(50)},
			stores: 20,
			want:   7,
		},
		{
			name:   "explicit zero disables",
			opts:   []memory.Option{memory.WithMaxEntries(50), memory.WithMaxEntriesPerSubject(0)},
			stores: 20,
			want:   20,
		},
		{
			name:   "explicit value above the global cap",
			opts:   []memory.Option{memory.WithMaxEntries(5), memory.WithMaxEntriesPerSubject(100)},
			stores: 20,
			want:   5,
		},
		{
			name:   "ignored negative keeps the derived cap",
			opts:   []memory.Option{memory.WithMaxEntries(50), memory.WithMaxEntriesPerSubject(-1)},
			stores: 20,
			want:   5,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := memory.New(time.Hour, tc.opts...)
			for i := range tc.stores {
				require.NoError(t, s.Store(t.Context(), fmt.Sprintf("a-%d", i), subjectMetadata("tenant-A")))
			}
			require.Equal(t, tc.want, s.Len())
		})
	}
}
