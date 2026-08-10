// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package denylist_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/denylist"
)

// epoch is the instant the targets' controllable clock starts at. A fixed base
// keeps a failure reproducible; a real clock would make the expiry boundary
// depend on how long the execution took.
var epoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// operation is one mutation the fuzzer can apply, decoded from a byte so the
// mutator can explore orderings rather than a fixed script.
type operation struct {
	kind   byte
	key    string
	offset time.Duration
}

// FuzzRevocationIsNeverForgottenEarly is the revocation oracle: a key that was
// revoked reports revoked, for as long as it was meant to.
//
// A denylist is what makes a signed token stop working before it expires — a
// leaked credential, a logged-out session, a compromised service identity. A
// sequence of revokes, timed revokes, restores and clock movement that leaves
// the list disagreeing with the operator's intent is a credential that keeps
// working, and nothing about the request will look wrong.
//
// The oracle is a plain map maintained alongside, so the answer never comes
// from the implementation being tested.
func FuzzRevocationIsNeverForgottenEarly(f *testing.F) {
	f.Add([]byte{0, 3}, "jti-1", "jti-2")
	f.Add([]byte{1, 4, 0, 2}, "a", "b")
	f.Add([]byte{2, 0, 1}, "same", "same")
	f.Add([]byte{}, "", "")
	f.Add([]byte{1, 1, 1, 1, 3, 3}, "x", "y")

	f.Fuzz(func(t *testing.T, script []byte, keyA, keyB string) {
		now := epoch
		list := denylist.New(denylist.WithClock(func() time.Time { return now }))

		// want mirrors the intended state: absent means allowed, the zero time
		// means revoked forever, anything else is the expiry.
		want := map[string]time.Time{}

		for i, op := range script {
			key := keyA
			if op&1 == 1 {
				key = keyB
			}

			switch (op >> 1) % 4 {
			case 0: // Permanent revoke.
				list.Revoke(key)
				want[key] = time.Time{}
			case 1: // Timed revoke, an hour out.
				expiry := now.Add(time.Hour)
				list.RevokeUntil(key, expiry)
				// An expiry at or before now is documented as a no-op; an hour
				// ahead never is.
				want[key] = expiry
			case 2: // Restore.
				list.Restore(key)
				delete(want, key)
			case 3: // Move the clock forward past some expiries.
				now = now.Add(40 * time.Minute)
			}

			for _, key := range []string{keyA, keyB} {
				expiry, revoked := want[key]
				expected := revoked && (expiry.IsZero() || now.Before(expiry))

				require.Equal(t, expected, list.IsRevoked(key),
					"step %d (op %d): key %q", i, op, key)
			}
		}
	})
}

// FuzzSweepDoesNotChangeAnswers pins that reclaiming memory is invisible.
//
// Sweep drops entries whose expiry has passed, and it also runs implicitly on
// every write. If it can drop an entry that is still in force — or leave one
// that should have lapsed — then whether a token is accepted depends on how
// many unrelated revocations happened to arrive first.
func FuzzSweepDoesNotChangeAnswers(f *testing.F) {
	f.Add("jti-1", int64(3600), true, int64(0))
	f.Add("jti-1", int64(-1), false, int64(7200))
	f.Add("", int64(0), true, int64(1))

	f.Fuzz(func(t *testing.T, key string, ttlSeconds int64, permanent bool, advanceSeconds int64) {
		now := epoch
		list := denylist.New(denylist.WithClock(func() time.Time { return now }))

		if permanent {
			list.Revoke(key)
		} else {
			list.RevokeUntil(key, now.Add(time.Duration(ttlSeconds)*time.Second))
		}

		now = now.Add(time.Duration(advanceSeconds%(1<<20)) * time.Second)

		before := list.IsRevoked(key)
		list.Sweep()
		require.Equal(t, before, list.IsRevoked(key),
			"Sweep changed the verdict for %q", key)
	})
}

// FuzzRestoreAlwaysWins pins the operator's escape hatch: whatever came before,
// a Restore re-allows the key.
//
// This is the undo for a revocation issued in error, and an implementation
// where a permanent revoke survived it would leave an identity locked out with
// no way back short of restarting the process.
func FuzzRestoreAlwaysWins(f *testing.F) {
	f.Add([]byte{0, 1, 0}, "jti-1")
	f.Add([]byte{1, 1}, "")
	f.Add([]byte{}, "k")

	f.Fuzz(func(t *testing.T, script []byte, key string) {
		now := epoch
		list := denylist.New(denylist.WithClock(func() time.Time { return now }))

		for _, op := range script {
			if op%2 == 0 {
				list.Revoke(key)
			} else {
				list.RevokeUntil(key, now.Add(time.Hour))
			}
		}

		list.Restore(key)
		require.False(t, list.IsRevoked(key), "a restored key stayed revoked: %q", key)
	})
}
