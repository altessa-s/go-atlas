// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package uowtest_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/messaging/eventbus/uow/uowtest"
)

type sliceStore struct {
	vals []string
}

func (s *sliceStore) Snapshot() func() {
	snap := slices.Clone(s.vals)
	return func() {
		s.vals = slices.Clone(snap)
	}
}

func TestFake_SnapshotIsolationAndRetries(t *testing.T) {
	t.Parallel()

	s1 := &sliceStore{vals: []string{"init"}}
	s2 := &sliceStore{vals: []string{"base"}}
	f := &uowtest.Fake{Attempts: 3, CommitRetries: 2, Isolate: []uowtest.Snapshotter{s1, s2}}

	err := f.WithTransaction(t.Context(), func(ctx context.Context) error {
		require.True(t, f.InTransaction(ctx))
		s1.vals = append(s1.vals, "step")
		s2.vals = append(s2.vals, "step")
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 3, f.Calls())
	require.Equal(t, 1, f.Txns())
	require.Equal(t, 2, f.CommitRetriesDone())
	require.Equal(t, 3, f.CommitAttempts())
	require.Equal(t, []string{"init", "step"}, s1.vals)
	require.Equal(t, []string{"base", "step"}, s2.vals)
	require.False(t, f.InTransaction(t.Context()))
}

func TestFake_Outcomes(t *testing.T) {
	t.Parallel()

	errBoom := errors.New("boom")
	tests := []struct {
		name    string
		fake    func() *uowtest.Fake
		fn      func(s *sliceStore) error
		wantErr error
		want    []string
	}{
		{
			name:    "callback error rolls back",
			fake:    func() *uowtest.Fake { return &uowtest.Fake{} },
			fn:      func(*sliceStore) error { return errBoom },
			wantErr: errBoom,
			want:    []string{"v0"},
		},
		{
			name:    "fail rolls back",
			fake:    func() *uowtest.Fake { return &uowtest.Fake{Fail: errBoom} },
			wantErr: errBoom,
			want:    []string{"v0"},
		},
		{
			name:    "abort rolls back",
			fake:    func() *uowtest.Fake { return &uowtest.Fake{Abort: func(int) error { return errBoom }} },
			wantErr: errBoom,
			want:    []string{"v0"},
		},
		{
			name:    "unknown keeps the commit",
			fake:    func() *uowtest.Fake { return &uowtest.Fake{Unknown: func(int) error { return errBoom }} },
			wantErr: errBoom,
			want:    []string{"v0", "v1"},
		},
		{
			name:    "recovered panic rolls back",
			fake:    func() *uowtest.Fake { return &uowtest.Fake{RecoverPanics: true} },
			fn:      func(*sliceStore) error { panic("boom") },
			wantErr: uowtest.ErrPanicked,
			want:    []string{"v0"},
		},
		{
			name: "commits",
			fake: func() *uowtest.Fake { return &uowtest.Fake{} },
			want: []string{"v0", "v1"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := &sliceStore{vals: []string{"v0"}}
			f := tc.fake()
			f.Isolate = []uowtest.Snapshotter{s}
			err := f.WithTransaction(t.Context(), func(context.Context) error {
				s.vals = append(s.vals, "v1")
				if tc.fn != nil {
					return tc.fn(s)
				}
				return nil
			})
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.want, s.vals)
		})
	}
}

func TestFake_PerTransactionScripts(t *testing.T) {
	t.Parallel()

	errBoom := errors.New("boom")
	failOn := func(n int) func(txn int) error {
		return func(txn int) error {
			if txn == n {
				return errBoom
			}
			return nil
		}
	}
	f := &uowtest.Fake{Abort: failOn(1), Unknown: failOn(2)}
	noop := func(context.Context) error { return nil }

	require.ErrorIs(t, f.WithTransaction(t.Context(), noop), errBoom)
	require.ErrorIs(t, f.WithTransaction(t.Context(), noop), errBoom)
	require.NoError(t, f.WithTransaction(t.Context(), noop))
	require.Equal(t, 3, f.Txns())
}

func TestFake_NestedCallJoins(t *testing.T) {
	t.Parallel()

	f := &uowtest.Fake{}
	err := f.WithTransaction(t.Context(), func(ctx context.Context) error {
		return f.WithTransaction(ctx, func(inner context.Context) error {
			require.True(t, f.InTransaction(inner))
			return nil
		})
	})
	require.NoError(t, err)
	require.Equal(t, 1, f.Txns())
	require.Equal(t, 1, f.Calls())
}

func TestFake_PanicPropagatesByDefault(t *testing.T) {
	t.Parallel()

	s := &sliceStore{vals: []string{"v0"}}
	f := &uowtest.Fake{Isolate: []uowtest.Snapshotter{s}}
	require.PanicsWithValue(t, "boom", func() {
		_ = f.WithTransaction(t.Context(), func(context.Context) error {
			s.vals = append(s.vals, "uncommitted")
			panic("boom")
		})
	})
	// The panicking attempt is rolled back and the mutex is released: a later transaction still runs.
	require.Equal(t, []string{"v0"}, s.vals)
	require.NoError(t, f.WithTransaction(t.Context(), func(context.Context) error { return nil }))
}

func TestFake_OtherFakeIsNotATransaction(t *testing.T) {
	t.Parallel()

	a, b := &uowtest.Fake{}, &uowtest.Fake{}
	require.NoError(t, a.WithTransaction(t.Context(), func(ctx context.Context) error {
		require.True(t, a.InTransaction(ctx))
		require.False(t, b.InTransaction(ctx))
		return nil
	}))
}

// A callback context kept after its attempt ended is no longer in a transaction: a later WithTransaction on it runs a full
// transaction (snapshots, rollback, failure injection) instead of joining a finished one.
func TestFake_RetainedContextIsNotInTransaction(t *testing.T) {
	t.Parallel()

	errBoom := errors.New("boom")
	tests := []struct {
		name string
		fake func() *uowtest.Fake
		fn   func(context.Context) error
	}{
		{name: "after commit", fake: func() *uowtest.Fake { return &uowtest.Fake{} }},
		{name: "after rollback", fake: func() *uowtest.Fake { return &uowtest.Fake{} }, fn: func(context.Context) error { return errBoom }},
		{name: "after a retried attempt", fake: func() *uowtest.Fake { return &uowtest.Fake{Attempts: 2} }},
		{name: "after a recovered panic", fake: func() *uowtest.Fake { return &uowtest.Fake{RecoverPanics: true} },
			fn: func(context.Context) error { panic("boom") }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := tc.fake()
			var kept []context.Context
			_ = f.WithTransaction(t.Context(), func(ctx context.Context) error {
				kept = append(kept, ctx)
				require.True(t, f.InTransaction(ctx))
				if tc.fn != nil {
					return tc.fn(ctx)
				}
				return nil
			})
			require.NotEmpty(t, kept)
			for _, ctx := range kept {
				require.False(t, f.InTransaction(ctx))
			}

			// A failing transaction started from a kept context is rolled back, not joined.
			s := &sliceStore{vals: []string{"v0"}}
			f.Fail, f.Isolate, f.Attempts, f.RecoverPanics = errBoom, []uowtest.Snapshotter{s}, 1, false
			err := f.WithTransaction(kept[0], func(context.Context) error {
				s.vals = append(s.vals, "uncommitted")
				return nil
			})
			require.ErrorIs(t, err, errBoom)
			require.Equal(t, []string{"v0"}, s.vals)
		})
	}
}
