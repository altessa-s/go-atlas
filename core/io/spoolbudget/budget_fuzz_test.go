// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package spoolbudget_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/io/spoolbudget"
)

// FuzzBudgetIsFullyReleased is the accounting oracle: whatever a spool reserved
// must come back when it is closed.
//
// The budget is a peak bound on bytes held across every in-flight spool, and it
// is enforced by a semaphore that a caller cannot inspect. A path that releases
// less than it reserved leaks capacity permanently — the service keeps working
// until, one deploy later, request bodies start blocking forever on a budget
// that has nothing left. That failure is impossible to attribute to the request
// that caused it, so the accounting has to be checked here.
//
// The check is indirect but exact: after N spools are opened and closed, a
// spool for the entire limit must still succeed, which it can only do if every
// byte came back.
func FuzzBudgetIsFullyReleased(f *testing.F) {
	f.Add(int64(1024), []byte("payload"), uint8(3), int64(64))
	f.Add(int64(16), []byte(""), uint8(1), int64(16))
	f.Add(int64(4096), bytes.Repeat([]byte("x"), 200), uint8(5), int64(256))

	f.Fuzz(func(t *testing.T, limit int64, payload []byte, rounds uint8, maxBytes int64) {
		if limit <= 0 || limit > 1<<20 {
			t.Skip("limits outside the plausible range say nothing about the accounting")
		}
		if maxBytes <= 0 || maxBytes > limit {
			t.Skip("a per-spool cap must be positive and fit the budget, as Spool documents")
		}

		budget := spoolbudget.New(limit)

		for range int(rounds) % 8 {
			sp, err := budget.Spool(t.Context(), bytes.NewReader(payload), maxBytes)
			if err != nil {
				// An oversize payload is refused before anything is held; the
				// reservation must still have been returned.
				continue
			}
			require.NoError(t, sp.Close())
		}

		// If any byte leaked, a full-limit reservation can no longer be
		// satisfied and this blocks until the context ends.
		full, err := budget.Spool(t.Context(), bytes.NewReader(nil), limit)
		require.NoError(t, err, "the budget did not return every byte after %d rounds", rounds%8)
		require.NoError(t, full.Close())
	})
}

// FuzzCloseIsIdempotent pins that releasing twice does not hand back capacity
// that was never held.
//
// Close is called from defers and from error paths, so a double Close is
// routine. Releasing twice would inflate the budget past its configured limit —
// the opposite failure to a leak, and the more dangerous one: the bound the
// budget exists to enforce silently stops holding.
func FuzzCloseIsIdempotent(f *testing.F) {
	f.Add(int64(1024), []byte("payload"), int64(64), uint8(2))
	f.Add(int64(32), []byte(""), int64(32), uint8(5))

	f.Fuzz(func(t *testing.T, limit int64, payload []byte, maxBytes int64, closes uint8) {
		if limit <= 0 || limit > 1<<20 {
			t.Skip("limits outside the plausible range say nothing about the accounting")
		}
		if maxBytes <= 0 || maxBytes > limit {
			t.Skip("a per-spool cap must be positive and fit the budget, as Spool documents")
		}

		budget := spoolbudget.New(limit)

		sp, err := budget.Spool(t.Context(), bytes.NewReader(payload), maxBytes)
		if err != nil {
			t.Skip("an oversize payload holds nothing to release")
		}

		for range max(int(closes)%4, 1) {
			require.NoError(t, sp.Close())
		}

		// The budget must be back to exactly its limit — no more, no less.
		full, err := budget.Spool(t.Context(), bytes.NewReader(nil), limit)
		require.NoError(t, err, "repeated Close lost capacity")
		require.NoError(t, full.Close())
	})
}

// FuzzDisabledBudgetImposesNoBound pins the documented nil-receiver behavior: a
// disabled budget is usable without a nil check and never blocks.
//
// Callers are told they may hold a nil *Budget and call it directly, which is
// what keeps the "budget optional" configuration from sprouting nil checks at
// every call site. A path that reserved against a disabled budget would
// deadlock on a semaphore that does not exist.
func FuzzDisabledBudgetImposesNoBound(f *testing.F) {
	f.Add(int64(0), []byte("payload"), int64(8))
	f.Add(int64(-1), []byte(""), int64(0))

	f.Fuzz(func(t *testing.T, limit int64, payload []byte, maxBytes int64) {
		if limit > 0 {
			t.Skip("this target is about the disabled budget")
		}
		if maxBytes < 0 || maxBytes > 1<<20 {
			t.Skip("an implausible cap says nothing about the disabled path")
		}

		budget := spoolbudget.New(limit)
		require.Zero(t, budget.Limit(), "a disabled budget must report no ceiling")

		sp, err := budget.Spool(t.Context(), bytes.NewReader(payload), maxBytes)
		if err != nil {
			require.Positive(t, maxBytes, "a spool with no cap was refused")
			require.Greater(t, int64(len(payload)), maxBytes)
			return
		}
		require.NoError(t, sp.Close())
		require.NoError(t, sp.Close(), "Close must stay safe to repeat")
	})
}
