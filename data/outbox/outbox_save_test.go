// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// savingStore records what actually reached the store, so the validation tests
// can assert that a rejected batch never got there.
type savingStore struct {
	recordingStore
	saved []Event
}

func (s *savingStore) SaveEvents(_ context.Context, events ...Event) error {
	s.saved = append(s.saved, events...)
	return nil
}

func TestSave_RejectsEmptyKey(t *testing.T) {
	t.Parallel()

	store := &savingStore{}
	ob := New(store, noopHandler)

	err := ob.Save(t.Context(), Event{Key: "", Payload: []byte("x")})

	require.ErrorIs(t, err, ErrEmptyKey)
	require.Empty(t, store.saved, "an invalid batch must not reach the store")
}

func TestSave_RejectsOversizedPayload(t *testing.T) {
	t.Parallel()

	store := &savingStore{}
	ob := New(store, noopHandler, WithMaxPayloadBytes(16))

	err := ob.Save(t.Context(), Event{Key: "k", Payload: bytes.Repeat([]byte("x"), 17)})

	require.ErrorIs(t, err, ErrPayloadTooLarge)
	require.Empty(t, store.saved)
}

// A whole batch is validated before anything is written: a caller running Save
// inside a transaction must be able to abort cleanly on the error.
func TestSave_ValidatesWholeBatchBeforeWriting(t *testing.T) {
	t.Parallel()

	store := &savingStore{}
	ob := New(store, noopHandler)

	err := ob.Save(t.Context(),
		Event{Key: "good", Payload: []byte("x")},
		Event{Key: "", Payload: []byte("x")},
	)

	require.ErrorIs(t, err, ErrEmptyKey)
	require.Empty(t, store.saved, "no event may be persisted when a later one is invalid")
}

func TestSave_AcceptsPayloadAtTheLimit(t *testing.T) {
	t.Parallel()

	store := &savingStore{}
	ob := New(store, noopHandler, WithMaxPayloadBytes(16))

	require.NoError(t, ob.Save(t.Context(), Event{Key: "k", Payload: bytes.Repeat([]byte("x"), 16)}))
	require.Len(t, store.saved, 1)
}

func TestSave_PayloadLimitDisabledByZero(t *testing.T) {
	t.Parallel()

	store := &savingStore{}
	ob := New(store, noopHandler, WithMaxPayloadBytes(0))

	require.NoError(t, ob.Save(t.Context(), Event{Key: "k", Payload: bytes.Repeat([]byte("x"), 2<<20)}))
	require.Len(t, store.saved, 1)
}
