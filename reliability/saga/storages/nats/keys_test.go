// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/nats/natsbase"
	"github.com/altessa-s/go-atlas/internal/nats/natskey/natskeytest"
	"github.com/altessa-s/go-atlas/reliability/saga"

	natsstore "github.com/altessa-s/go-atlas/reliability/saga/storages/nats"
)

// TestStore_IDsNATSRejectsVerbatim runs the instance lifecycle and the
// recovery scan on IDs the server rejects as they are. The scan reads the
// listed physical keys back as they are, so it must find every instance.
func TestStore_IDsNATSRejectsVerbatim(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := t.Context()
	ids := natskeytest.Keys()

	for _, id := range ids {
		require.NoError(t, s.Create(ctx, &saga.Instance{ID: id, Definition: id, Status: saga.StatusCompensating}), id)
	}

	recoverable, err := s.FetchRecoverable(ctx, "", time.Now(), 0)
	require.NoError(t, err)
	found := make([]string, 0, len(recoverable))
	for _, inst := range recoverable {
		found = append(found, inst.ID)
	}
	require.ElementsMatch(t, ids, found)

	for _, id := range ids {
		got, err := s.Get(ctx, id)
		require.NoError(t, err, id)
		require.Equal(t, id, got.Definition, id)

		got.Status = saga.StatusCompleted
		require.NoError(t, s.Update(ctx, got), id)

		require.NoError(t, s.Delete(ctx, id), id)
		_, err = s.Get(ctx, id)
		require.ErrorIs(t, err, saga.ErrInstanceNotFound, id)
	}
}

func TestStore_ValidIDIsStoredUnderItself(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := t.Context()

	require.NoError(t, s.Create(ctx, &saga.Instance{ID: natskeytest.PlainKey, Status: saga.StatusRunning}))

	_, err := s.KV().Get(ctx, natskeytest.PlainKey)
	require.NoError(t, err)
}

func TestStore_IDTooLong(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	id := strings.Repeat(":", 2000)

	require.ErrorIs(t, s.Create(t.Context(), &saga.Instance{ID: id}), natsstore.ErrKeyTooLong)
	_, err := s.Get(t.Context(), id)
	require.ErrorIs(t, err, natsstore.ErrKeyTooLong)
	require.ErrorIs(t, s.Update(t.Context(), &saga.Instance{ID: id, Version: 1}), natsstore.ErrKeyTooLong)
	require.ErrorIs(t, s.Delete(t.Context(), id), natsstore.ErrKeyTooLong)
}

// TestStore_InvalidUTF8IDIsRejected guards the instance identity: JSON would
// store "\xff" as U+FFFD, so the instance could not be found again by its ID.
func TestStore_InvalidUTF8IDIsRejected(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	id := "order\xff"

	require.ErrorIs(t, s.Create(t.Context(), &saga.Instance{ID: id}), natsstore.ErrInvalidID)
	_, err := s.Get(t.Context(), id)
	require.ErrorIs(t, err, natsstore.ErrInvalidID)
	require.ErrorIs(t, s.Update(t.Context(), &saga.Instance{ID: id, Version: 1}), natsstore.ErrInvalidID)
	require.ErrorIs(t, s.Delete(t.Context(), id), natsstore.ErrInvalidID)
}

type invalidKeyKV struct{ jetstream.KeyValue }

func (invalidKeyKV) Get(context.Context, string) (jetstream.KeyValueEntry, error) {
	return nil, jetstream.ErrInvalidKey
}

func TestStoreInvalidStorageKey(t *testing.T) {
	t.Parallel()
	s := &natsstore.Store{Base: natsbase.NewBase(invalidKeyKV{})}
	for _, tc := range []struct {
		name, id string
		cause    error
	}{
		{"encoded length", strings.Repeat(":", 2000), natsstore.ErrKeyTooLong},
		{"invalid UTF-8", "order\xff", natsstore.ErrInvalidID},
		{"client rejection", "valid", jetstream.ErrInvalidKey},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := s.Get(t.Context(), tc.id)
			require.ErrorIs(t, err, saga.ErrInvalidStorageKey)
			require.ErrorIs(t, err, tc.cause)
		})
	}
}
