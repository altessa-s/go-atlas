// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/encoding/serializer"
	"github.com/altessa-s/go-atlas/data/idempotency/storages"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func TestAttemptLock_New(t *testing.T) {
	s := testhelpers.NewMockIdempotencyStorage()
	k := New(s)
	ctx := t.Context()

	ok, state, err := k.AttemptLock(ctx, "key1")
	require.NoError(t, err)
	require.True(t, ok, "expected lock acquired")
	require.NotNil(t, state, "AttemptLock now returns a non-nil State carrying the lock token on acquire")
	require.Equal(t, storages.StatusInProgress, state.Status)
	require.NotNil(t, state.LockToken(), "lock token must be populated for Complete to succeed")
}

func TestAttemptLock_InProgress(t *testing.T) {
	s := testhelpers.NewMockIdempotencyStorage()
	k := New(s)
	ctx := t.Context()

	_, _, _ = k.AttemptLock(ctx, "key1")

	ok, state, err := k.AttemptLock(ctx, "key1")
	require.NoError(t, err)
	require.False(t, ok, "expected lock NOT acquired (already in progress)")
	require.NotNil(t, state)
	require.Equal(t, storages.StatusInProgress, state.Status)
}

func TestAttemptLock_Completed(t *testing.T) {
	s := testhelpers.NewMockIdempotencyStorage()
	k := New(s)
	ctx := t.Context()

	_, lockState, _ := k.AttemptLock(ctx, "key1")
	_ = k.Complete(ctx, "key1", map[string]string{"result": "ok"}, lockState)

	ok, state, err := k.AttemptLock(ctx, "key1")
	require.NoError(t, err)
	require.False(t, ok, "expected lock NOT acquired (completed)")
	require.NotNil(t, state)
	require.Equal(t, storages.StatusSuccess, state.Status)
}

func TestComplete(t *testing.T) {
	s := testhelpers.NewMockIdempotencyStorage()
	k := New(s)
	ctx := t.Context()

	_, lockState, _ := k.AttemptLock(ctx, "key1")
	err := k.Complete(ctx, "key1", "result-data", lockState)
	require.NoError(t, err)
}

func TestDelete(t *testing.T) {
	s := testhelpers.NewMockIdempotencyStorage()
	k := New(s)
	ctx := t.Context()

	_, _, _ = k.AttemptLock(ctx, "key1")
	err := k.Delete(ctx, "key1")
	require.NoError(t, err)

	// After delete, lock should succeed again.
	ok, _, err := k.AttemptLock(ctx, "key1")
	require.NoError(t, err)
	require.True(t, ok, "expected lock after delete")
}

func TestAttemptLock_EmptyKey(t *testing.T) {
	s := testhelpers.NewMockIdempotencyStorage()
	k := New(s)

	ok, state, err := k.AttemptLock(t.Context(), "")
	require.ErrorIs(t, err, ErrEmptyKey,
		"empty key must surface ErrEmptyKey instead of silently succeeding (which would disable dedupe)")
	require.False(t, ok)
	require.Nil(t, state)
}

func TestStorageFunc(t *testing.T) {
	called := false
	sf := StorageFunc{
		AttemptLockFunc: func(_ context.Context, _ string, _ []byte) (bool, []byte, []byte, error) {
			called = true
			return true, nil, nil, nil
		},
		CompleteFunc: func(_ context.Context, _ string, _ []byte, _ []byte) error { return nil },
		DeleteFunc:   func(_ context.Context, _ string) error { return nil },
	}

	_, _, _, _ = sf.AttemptLock(t.Context(), "key", nil)
	require.True(t, called, "AttemptLockFunc not called")
	_ = sf.Complete(t.Context(), "key", nil, nil)
	_ = sf.Delete(t.Context(), "key")
}

// TestComplete_NilLockState verifies that Complete refuses to write
// without the *State returned by AttemptLock — passing nil silently
// would defeat the stolen-lock guard.
func TestComplete_NilLockState(t *testing.T) {
	s := testhelpers.NewMockIdempotencyStorage()
	k := New(s)

	err := k.Complete(t.Context(), "key", "data", nil)
	require.ErrorIs(t, err, ErrMissingLockState)
}

// TestAttemptLockWithOpts_PassedToStorage verifies that the Keeper
// forwards opts.LockTTL unchanged to Storage.AttemptLockWithTTL and
// that plain AttemptLock calls through with ttl=0.
func TestAttemptLockWithOpts_PassedToStorage(t *testing.T) {
	var lastTtl time.Duration
	sf := StorageFunc{
		AttemptLockFunc: func(_ context.Context, _ string, _ []byte) (bool, []byte, []byte, error) {
			return true, nil, []byte("token"), nil
		},
		AttemptLockWithTTLFunc: func(_ context.Context, _ string, _ []byte, lockTtl time.Duration) (bool, []byte, []byte, error) {
			lastTtl = lockTtl
			return true, nil, []byte("token"), nil
		},
		CompleteFunc: func(_ context.Context, _ string, _ []byte, _ []byte) error { return nil },
		DeleteFunc:   func(_ context.Context, _ string) error { return nil },
	}
	k := New(sf)

	_, _, err := k.AttemptLockWithOpts(t.Context(), "key1", AttemptLockOpts{LockTTL: 7 * time.Second})
	require.NoError(t, err)
	require.Equal(t, 7*time.Second, lastTtl, "Keeper must forward opts.LockTTL unchanged")

	_, _, err = k.AttemptLock(t.Context(), "key2")
	require.NoError(t, err)
	require.Equal(t, time.Duration(0), lastTtl,
		"plain AttemptLock must call AttemptLockWithTTL with ttl=0")
}

// TestAttemptLock_OrphanSteal_KeeperDefault verifies that a stale
// InProgress entry (older than the Keeper's MaxLockDuration) is
// reclaimed by the next AttemptLock via storage.Steal.
func TestAttemptLock_OrphanSteal_KeeperDefault(t *testing.T) {
	t.Parallel()
	s := testhelpers.NewMockIdempotencyStorage()
	k := New(s, WithMaxLockDuration(50*time.Millisecond))
	ctx := t.Context()

	// Holder A acquires the lock, then "crashes" — never calls Complete/Delete.
	okA, stateA, err := k.AttemptLock(ctx, "key-orphan")
	require.NoError(t, err)
	require.True(t, okA)
	require.NotNil(t, stateA.LockToken())

	// Forge LockedAt into the past so the next AttemptLock sees an
	// orphan beyond the Keeper's MaxLockDuration without sleeping.
	forgePastLockedAt(t, s, "key-orphan", 10*time.Minute)

	okB, stateB, err := k.AttemptLock(ctx, "key-orphan")
	require.NoError(t, err)
	require.True(t, okB, "stale orphan must be reclaimed by AttemptLock")
	require.NotNil(t, stateB)
	require.Equal(t, storages.StatusInProgress, stateB.Status)
	require.NotNil(t, stateB.LockToken(), "fresh lock token must be carried on the stolen state")
	require.NotEqual(t, stateA.LockToken(), stateB.LockToken(),
		"stolen lock token must differ from the orphan's token")

	// Holder A's stale Complete must now surface ErrLockStolen — its
	// CAS token is no longer current.
	err = k.Complete(ctx, "key-orphan", "result-from-A", stateA)
	require.ErrorIs(t, err, ErrLockStolen,
		"crashed holder's Complete must not overwrite the new owner")

	// Holder B's Complete must succeed.
	require.NoError(t, k.Complete(ctx, "key-orphan", "result-from-B", stateB))
}

// TestAttemptLock_OrphanSteal_PerCallOverride verifies that
// AttemptLockOpts.MaxLockDuration overrides the Keeper-wide value.
func TestAttemptLock_OrphanSteal_PerCallOverride(t *testing.T) {
	t.Parallel()
	s := testhelpers.NewMockIdempotencyStorage()
	// Keeper-wide threshold is huge; per-call override is tight.
	k := New(s, WithMaxLockDuration(time.Hour))
	ctx := t.Context()

	_, _, err := k.AttemptLock(ctx, "key1")
	require.NoError(t, err)
	forgePastLockedAt(t, s, "key1", 30*time.Second)

	// Without override, the Keeper would say "in progress" because the
	// entry is well within the 1h threshold.
	ok, state, err := k.AttemptLock(ctx, "key1")
	require.NoError(t, err)
	require.False(t, ok, "Keeper-wide MaxLockDuration would not steal here")
	require.Equal(t, storages.StatusInProgress, state.Status)

	// With a tight per-call override, the same entry is now an orphan.
	ok, state, err = k.AttemptLockWithOpts(ctx, "key1", AttemptLockOpts{MaxLockDuration: 5 * time.Second})
	require.NoError(t, err)
	require.True(t, ok, "per-call MaxLockDuration must override the Keeper default")
	require.NotNil(t, state.LockToken())
}

// TestAttemptLock_OrphanSteal_FreshLockNotStolen verifies that a
// fresh InProgress entry is NOT reclaimed even when MaxLockDuration is
// configured — orphan-steal is gated by the LockedAt timestamp.
func TestAttemptLock_OrphanSteal_FreshLockNotStolen(t *testing.T) {
	t.Parallel()
	s := testhelpers.NewMockIdempotencyStorage()
	k := New(s, WithMaxLockDuration(time.Second))
	ctx := t.Context()

	_, stateA, err := k.AttemptLock(ctx, "key1")
	require.NoError(t, err)

	// Don't touch LockedAt — the entry is fresh.
	ok, state, err := k.AttemptLock(ctx, "key1")
	require.NoError(t, err)
	require.False(t, ok, "fresh InProgress entry must not be stolen")
	require.Equal(t, storages.StatusInProgress, state.Status)
	require.Nil(t, state.LockToken(),
		"observer state for a non-acquired lock must not carry a token")

	require.NoError(t, k.Complete(ctx, "key1", "ok", stateA))
}

// TestAttemptLock_OrphanSteal_Disabled verifies that
// MaxLockDuration <= 0 disables the orphan-reclaim path entirely —
// stale entries surface as in-progress to the caller.
func TestAttemptLock_OrphanSteal_Disabled(t *testing.T) {
	t.Parallel()
	s := testhelpers.NewMockIdempotencyStorage()
	// WithMaxLockDuration rejects non-positive values, so we have to
	// force the field manually via the Keeper internals.
	k := New(s)
	k.opts.maxLockDuration = 0 // disable orphan reclaim
	ctx := t.Context()

	_, _, err := k.AttemptLock(ctx, "key1")
	require.NoError(t, err)
	forgePastLockedAt(t, s, "key1", time.Hour)

	ok, _, err := k.AttemptLock(ctx, "key1")
	require.NoError(t, err)
	require.False(t, ok, "MaxLockDuration<=0 must keep orphan-reclaim disabled")
}

// TestNew_WarnsOnDefaultMaxLockDuration verifies the soft-acknowledgment
// contract: New emits a slog.Warn when the caller does not pass
// WithMaxLockDuration, prompting service owners to evaluate the
// default for their workload.
func TestNew_WarnsOnDefaultMaxLockDuration(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	k := New(testhelpers.NewMockIdempotencyStorage(), WithLogger(logger))

	got := buf.String()
	require.Contains(t, got, "level=WARN", "expected a WARN-level log line")
	require.Contains(t, got, "using default MaxLockDuration",
		"warning must mention MaxLockDuration so owners can grep for it")
	require.Equal(t, DefaultMaxLockDuration, k.opts.maxLockDuration,
		"unset maxLockDuration must be normalized to DefaultMaxLockDuration after warn")
}

// TestNew_NoWarnWhenOverridden verifies that an explicit non-default
// override silences the warning.
func TestNew_NoWarnWhenOverridden(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	override := 90 * time.Second
	k := New(testhelpers.NewMockIdempotencyStorage(),
		WithLogger(logger),
		WithMaxLockDuration(override))

	require.Empty(t, strings.TrimSpace(buf.String()),
		"explicit WithMaxLockDuration must silence the default warning")
	require.Equal(t, override, k.opts.maxLockDuration)
}

// TestNew_NoWarnWhenExplicitDefault is the "I read the docs, 5m is
// fine" escape hatch: passing the constant itself acknowledges the
// default and silences the warning.
func TestNew_NoWarnWhenExplicitDefault(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	k := New(testhelpers.NewMockIdempotencyStorage(),
		WithLogger(logger),
		WithMaxLockDuration(DefaultMaxLockDuration))

	require.Empty(t, strings.TrimSpace(buf.String()),
		"WithMaxLockDuration(DefaultMaxLockDuration) must be treated as explicit acknowledgment")
	require.Equal(t, DefaultMaxLockDuration, k.opts.maxLockDuration)
}

// forgePastLockedAt rewrites the InProgress wire stored under key so
// that LockedAt is `age` in the past. Used to exercise orphan-steal
// without sleeping in tests.
func forgePastLockedAt(t *testing.T, s *testhelpers.MockIdempotencyStorage, key string, age time.Duration) {
	t.Helper()
	raw, ok := s.Entry(key)
	require.True(t, ok, "key must exist in storage to forge LockedAt")

	ser := &serializer.JSON{}

	var wire serializedState
	require.NoError(t, ser.Deserialize(raw, &wire),
		"forged wire must round-trip through the keeper's serializer")
	wire.LockedAt = time.Now().Add(-age)

	forged, err := ser.Serialize(wire)
	require.NoError(t, err)
	s.SetEntry(key, forged)
}

// TestComplete_StolenLock verifies that ErrLockStolen from the
// underlying Storage propagates unchanged through the Keeper layer.
func TestComplete_StolenLock(t *testing.T) {
	s := testhelpers.NewMockIdempotencyStorage()
	k := New(s)
	ctx := t.Context()

	_, lockStateA, err := k.AttemptLock(ctx, "key1")
	require.NoError(t, err)
	require.NotNil(t, lockStateA)

	// Simulate B taking over the key after A's TTL expired.
	require.NoError(t, k.Delete(ctx, "key1"))
	_, _, err = k.AttemptLock(ctx, "key1")
	require.NoError(t, err)

	err = k.Complete(ctx, "key1", "result-from-A", lockStateA)
	require.ErrorIs(t, err, ErrLockStolen,
		"stale Complete must surface ErrLockStolen rather than overwriting the new holder")
}
