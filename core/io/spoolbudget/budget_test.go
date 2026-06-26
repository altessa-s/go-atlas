// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package spoolbudget_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/io/spool"
	"github.com/altessa-s/go-atlas/core/io/spoolbudget"
)

func TestBudget_DisabledIsPassthrough(t *testing.T) {
	t.Parallel()

	b := spoolbudget.New(0) // disabled => nil
	require.Nil(t, b)
	require.Zero(t, b.Limit())

	sp, err := b.Spool(t.Context(), strings.NewReader("hello"), 0)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sp.Close() })
	require.Equal(t, int64(5), sp.Size())
}

func TestBudget_ChargesAndReleases(t *testing.T) {
	t.Parallel()

	b := spoolbudget.New(10)
	require.Equal(t, int64(10), b.Limit())

	first, err := b.Spool(t.Context(), strings.NewReader(strings.Repeat("x", 6)), 6)
	require.NoError(t, err)

	// 6 of 10 are held; a second 6-byte spool cannot fit and blocks until ctx.
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	_, err = b.Spool(ctx, strings.NewReader(strings.Repeat("y", 6)), 6)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	// Releasing the first reservation frees the budget for the next spool.
	require.NoError(t, first.Close())
	second, err := b.Spool(t.Context(), strings.NewReader(strings.Repeat("z", 6)), 6)
	require.NoError(t, err)
	require.NoError(t, second.Close())
}

func TestBudget_BlockedSpoolUnblocksOnRelease(t *testing.T) {
	t.Parallel()

	b := spoolbudget.New(8)
	held, err := b.Spool(t.Context(), strings.NewReader(strings.Repeat("a", 8)), 8)
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() {
		sp, serr := b.Spool(t.Context(), strings.NewReader(strings.Repeat("b", 8)), 8)
		if serr == nil {
			_ = sp.Close()
		}
		done <- serr
	}()

	// The waiter must still be blocked while the full budget is held.
	select {
	case <-done:
		t.Fatal("spool acquired while the budget was exhausted")
	case <-time.After(50 * time.Millisecond):
	}

	require.NoError(t, held.Close())
	select {
	case serr := <-done:
		require.NoError(t, serr)
	case <-time.After(2 * time.Second):
		t.Fatal("spool did not acquire after the budget was released")
	}
}

func TestBudget_EmptySpoolHoldsNothing(t *testing.T) {
	t.Parallel()

	b := spoolbudget.New(4)
	// An empty source charges nothing, so a full-budget spool still fits.
	empty, err := b.Spool(t.Context(), strings.NewReader(""), 4)
	require.NoError(t, err)
	t.Cleanup(func() { _ = empty.Close() })

	full, err := b.Spool(t.Context(), strings.NewReader(strings.Repeat("c", 4)), 4)
	require.NoError(t, err)
	require.NoError(t, full.Close())
}

func TestBudget_OversizeReleasesReservation(t *testing.T) {
	t.Parallel()

	b := spoolbudget.New(4)
	// The reader exceeds the per-spool cap, so spool.New fails with ErrTooLarge
	// and the up-front reservation must be released rather than leaked.
	_, err := b.Spool(t.Context(), strings.NewReader(strings.Repeat("x", 5)), 4)
	require.ErrorIs(t, err, spool.ErrTooLarge)

	// Had the reservation leaked, this full-budget spool would block forever.
	next, err := b.Spool(t.Context(), strings.NewReader(strings.Repeat("y", 4)), 4)
	require.NoError(t, err)
	require.NoError(t, next.Close())
}
