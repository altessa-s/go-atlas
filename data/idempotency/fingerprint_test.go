// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/encoding/keys"
	"github.com/altessa-s/go-atlas/data/idempotency"
	"github.com/altessa-s/go-atlas/data/idempotency/storages/memory"
)

type ownerKey struct{}

func withOwner(ctx context.Context, owner string) context.Context {
	return context.WithValue(ctx, ownerKey{}, owner)
}

func ownerOf(ctx context.Context) string {
	owner, _ := ctx.Value(ownerKey{}).(string)
	return owner
}

func newKeeper(opts ...idempotency.Option) *idempotency.Keeper {
	return idempotency.New(memory.New(), append([]idempotency.Option{
		idempotency.WithMaxLockDuration(time.Minute),
	}, opts...)...)
}

func TestFingerprintMismatch(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	k := newKeeper()
	fpA := keys.Digest([]byte("create"), []byte(`{"amount":1}`))
	fpB := keys.Digest([]byte("create"), []byte(`{"amount":2}`))

	ok, held, err := k.AttemptLockWithOpts(ctx, "key", idempotency.AttemptLockOpts{Fingerprint: fpA})
	require.NoError(t, err)
	require.True(t, ok)

	// In progress: the same command waits, another command is refused.
	ok, st, err := k.AttemptLockWithOpts(ctx, "key", idempotency.AttemptLockOpts{Fingerprint: fpA})
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, idempotency.StatusInProgress, st.Status)
	_, st, err = k.AttemptLockWithOpts(ctx, "key", idempotency.AttemptLockOpts{Fingerprint: fpB})
	require.ErrorIs(t, err, idempotency.ErrFingerprintMismatch)
	require.Nil(t, st)

	// Completed: the fingerprint survives Complete.
	require.NoError(t, k.Complete(ctx, "key", "entity-1", held))
	_, st, err = k.AttemptLockWithOpts(ctx, "key", idempotency.AttemptLockOpts{Fingerprint: fpA})
	require.NoError(t, err)
	require.Equal(t, idempotency.StatusSuccess, st.Status)
	require.Equal(t, "entity-1", st.Data)
	require.Equal(t, fpA, st.Fingerprint())
	_, _, err = k.AttemptLockWithOpts(ctx, "key", idempotency.AttemptLockOpts{Fingerprint: fpB})
	require.ErrorIs(t, err, idempotency.ErrFingerprintMismatch)

	// A caller without a fingerprint is not checked.
	_, st, err = k.AttemptLock(ctx, "key")
	require.NoError(t, err)
	require.Equal(t, idempotency.StatusSuccess, st.Status)
}

func TestFingerprintNotRecorded(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	k := newKeeper()
	ok, held, err := k.AttemptLock(ctx, "legacy")
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, k.Complete(ctx, "legacy", nil, held))

	_, st, err := k.AttemptLockWithOpts(ctx, "legacy", idempotency.AttemptLockOpts{Fingerprint: []byte("fp")})
	require.NoError(t, err, "a record without a fingerprint is not checked")
	require.Equal(t, idempotency.StatusSuccess, st.Status)
}

func TestOwnerMismatch(t *testing.T) {
	t.Parallel()

	k := newKeeper(idempotency.WithOwner(ownerOf))
	alice, bob := withOwner(t.Context(), "alice"), withOwner(t.Context(), "bob")

	ok, held, err := k.AttemptLock(alice, "key")
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, k.Complete(alice, "key", "secret-entity", held))

	_, st, err := k.AttemptLock(bob, "key")
	require.ErrorIs(t, err, idempotency.ErrOwnerMismatch)
	require.Nil(t, st, "nothing of the record reaches another owner")

	_, st, err = k.AttemptLock(alice, "key")
	require.NoError(t, err)
	require.Equal(t, "secret-entity", st.Data)
	require.Equal(t, "alice", st.Owner())
}

func TestOwnerCheckedBeforeFingerprint(t *testing.T) {
	t.Parallel()

	k := newKeeper(idempotency.WithOwner(ownerOf))
	ok, _, err := k.AttemptLockWithOpts(withOwner(t.Context(), "alice"), "key", idempotency.AttemptLockOpts{Fingerprint: []byte("a")})
	require.NoError(t, err)
	require.True(t, ok)

	_, _, err = k.AttemptLockWithOpts(withOwner(t.Context(), "bob"), "key", idempotency.AttemptLockOpts{Fingerprint: []byte("b")})
	require.ErrorIs(t, err, idempotency.ErrOwnerMismatch, "another owner learns nothing about the fingerprint")
}

func TestReplayAuthorizer(t *testing.T) {
	t.Parallel()

	errForbidden := errors.New("forbidden")
	k := newKeeper(
		idempotency.WithOwner(ownerOf),
		idempotency.WithReplayAuthorizer(func(ctx context.Context, owner string) error {
			if ownerOf(ctx) == "admin" {
				return nil
			}
			return errForbidden
		}),
	)
	ok, held, err := k.AttemptLock(withOwner(t.Context(), "alice"), "key")
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, k.Complete(withOwner(t.Context(), "alice"), "key", "e", held))

	_, st, err := k.AttemptLock(withOwner(t.Context(), "admin"), "key")
	require.NoError(t, err)
	require.Equal(t, "e", st.Data)

	_, _, err = k.AttemptLock(withOwner(t.Context(), "bob"), "key")
	require.ErrorIs(t, err, errForbidden)
	require.ErrorIs(t, err, idempotency.ErrOwnerMismatch, "transports recognize the refusal")

	// The authorizer alone decides: it may refuse the record's own owner.
	_, _, err = k.AttemptLock(withOwner(t.Context(), "alice"), "key")
	require.ErrorIs(t, err, errForbidden, "the authorizer decides every replay")
}

func TestOwnerBlocksOrphanSteal(t *testing.T) {
	t.Parallel()

	k := newKeeper(idempotency.WithOwner(ownerOf))
	ok, _, err := k.AttemptLock(withOwner(t.Context(), "alice"), "key")
	require.NoError(t, err)
	require.True(t, ok)

	// The lock is orphaned for any reclaim threshold, yet another owner
	// may not take it over.
	_, _, err = k.AttemptLockWithOpts(withOwner(t.Context(), "bob"), "key", idempotency.AttemptLockOpts{MaxLockDuration: time.Nanosecond})
	require.ErrorIs(t, err, idempotency.ErrOwnerMismatch)
	ok, _, err = k.AttemptLockWithOpts(withOwner(t.Context(), "alice"), "key", idempotency.AttemptLockOpts{MaxLockDuration: time.Nanosecond})
	require.NoError(t, err)
	require.True(t, ok, "the owner reclaims its own orphaned lock")
}

func TestAuthorizedReaderCannotTakeOver(t *testing.T) {
	t.Parallel()

	k := newKeeper(
		idempotency.WithOwner(ownerOf),
		idempotency.WithReplayAuthorizer(func(context.Context, string) error { return nil }),
	)
	ok, _, err := k.AttemptLock(withOwner(t.Context(), "alice"), "key")
	require.NoError(t, err)
	require.True(t, ok)

	ok, st, err := k.AttemptLockWithOpts(withOwner(t.Context(), "admin"), "key", idempotency.AttemptLockOpts{MaxLockDuration: time.Nanosecond})
	require.NoError(t, err)
	require.False(t, ok, "reading another owner's record does not grant taking over its lock")
	require.Equal(t, idempotency.StatusInProgress, st.Status)
}

func TestTakeOverKeepsFingerprintAndOwner(t *testing.T) {
	t.Parallel()

	k := newKeeper()
	fpA := []byte("fingerprint-a")
	ok, _, err := k.AttemptLockWithOpts(t.Context(), "key", idempotency.AttemptLockOpts{Fingerprint: fpA})
	require.NoError(t, err)
	require.True(t, ok)

	// A takeover without a fingerprint must not erase the recorded one.
	ok, held, err := k.AttemptLockWithOpts(t.Context(), "key", idempotency.AttemptLockOpts{MaxLockDuration: time.Nanosecond})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, fpA, held.Fingerprint())
	require.NoError(t, k.Complete(t.Context(), "key", nil, held))

	_, _, err = k.AttemptLockWithOpts(t.Context(), "key", idempotency.AttemptLockOpts{Fingerprint: []byte("fingerprint-b")})
	require.ErrorIs(t, err, idempotency.ErrFingerprintMismatch)
}

func TestFingerprintNotAliased(t *testing.T) {
	t.Parallel()

	k := newKeeper()
	buf := []byte("fingerprint-a")
	ok, held, err := k.AttemptLockWithOpts(t.Context(), "key", idempotency.AttemptLockOpts{Fingerprint: buf})
	require.NoError(t, err)
	require.True(t, ok)
	copy(buf, "FINGERPRINT-B") // the caller reuses its buffer
	held.Fingerprint()[0] = 'X'
	require.NoError(t, k.Complete(t.Context(), "key", nil, held))

	_, st, err := k.AttemptLockWithOpts(t.Context(), "key", idempotency.AttemptLockOpts{Fingerprint: []byte("fingerprint-a")})
	require.NoError(t, err)
	require.Equal(t, idempotency.StatusSuccess, st.Status, "the fingerprint captured at acquisition is kept")
}

// TestReplayAuthorizerDecidesEveryReplay pins the access-check rule: the
// replay goes to whoever may read the record, decided from its owner label.
func TestReplayAuthorizerDecidesEveryReplay(t *testing.T) {
	t.Parallel()

	errNotFound := errors.New("not found")
	var seen []string
	k := newKeeper(
		idempotency.WithOwner(ownerOf),
		idempotency.WithReplayAuthorizer(func(ctx context.Context, owner string) error {
			seen = append(seen, owner)
			if ownerOf(ctx) == "auditor" || ownerOf(ctx) == owner {
				return nil
			}
			return errNotFound
		}),
	)
	ok, held, err := k.AttemptLock(withOwner(t.Context(), "le-1"), "key")
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, k.Complete(withOwner(t.Context(), "le-1"), "key", "payment-7", held))

	_, st, err := k.AttemptLock(withOwner(t.Context(), "le-1"), "key")
	require.NoError(t, err)
	require.Equal(t, "payment-7", st.Data)
	_, st, err = k.AttemptLock(withOwner(t.Context(), "auditor"), "key")
	require.NoError(t, err, "a reader other than the owner gets the replay")
	require.Equal(t, "payment-7", st.Data)
	_, st, err = k.AttemptLock(withOwner(t.Context(), "le-2"), "key")
	require.ErrorIs(t, err, errNotFound, "an invisible record reads as the service's NotFound")
	require.Nil(t, st)
	require.Equal(t, []string{"le-1", "le-1", "le-1"}, seen, "the authorizer gets the recorded label every time")
}
