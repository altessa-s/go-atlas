// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// fakeCursorStorage is a minimal in-memory CursorStorage for white-box tests
// (the memory subpackage imports this package, so it can't be used here).
type fakeCursorStorage struct {
	entries map[string]*CursorMetadata
}

func newFakeCursorStorage() *fakeCursorStorage {
	return &fakeCursorStorage{entries: map[string]*CursorMetadata{}}
}

func (s *fakeCursorStorage) Store(_ context.Context, key string, m *CursorMetadata) error {
	s.entries[key] = m
	return nil
}

func (s *fakeCursorStorage) Load(_ context.Context, key string) (*CursorMetadata, error) {
	m, ok := s.entries[key]
	if !ok {
		return nil, ErrCursorNotFound
	}
	return m, nil
}

func (s *fakeCursorStorage) Delete(_ context.Context, key string) error {
	delete(s.entries, key)
	return nil
}

func TestCursorMetadata_ValidateSubject(t *testing.T) {
	t.Parallel()

	t.Run("unbound cursor allows any subject", func(t *testing.T) {
		t.Parallel()
		m := &CursorMetadata{} // Subject == ""
		require.NoError(t, m.ValidateSubject("anyone"))
		require.NoError(t, m.ValidateSubject(""))
	})

	t.Run("bound cursor allows the same subject", func(t *testing.T) {
		t.Parallel()
		m := &CursorMetadata{Subject: "tenant-A"}
		require.NoError(t, m.ValidateSubject("tenant-A"))
	})

	t.Run("bound cursor rejects a different subject", func(t *testing.T) {
		t.Parallel()
		m := &CursorMetadata{Subject: "tenant-A"}
		require.ErrorIs(t, m.ValidateSubject("tenant-B"), ErrCursorSubjectMismatch)
		require.ErrorIs(t, m.ValidateSubject(""), ErrCursorSubjectMismatch)
	})
}

// TestStatefulCursor_SubjectBinding pins the fix end to end: a stateful cursor
// token minted for one subject cannot be replayed by another to continue
// paging, while the original subject continues normally.
func TestStatefulCursor_SubjectBinding(t *testing.T) {
	t.Parallel()

	storage := newFakeCursorStorage()
	sort := bson.D{{Key: "_id", Value: 1}}
	filter := bson.M{"status": "active"}
	cursorID := bson.NewObjectID().Hex()

	// Tenant A mints a "next" token bound to its identity.
	token, err := generateNextCursorToken(t.Context(), cursorID, sort, "_id", filter, storage, nil, "tenant-A")
	require.NoError(t, err)
	require.NotEmpty(t, token)
	require.Equal(t, "tenant-A", storage.entries[token].Subject)

	// Tenant A can continue paging with the same filter.
	cur, err := parseCursorToken(t.Context(), token, storage, filter, "tenant-A")
	require.NoError(t, err)
	require.NotNil(t, cur)

	// Tenant B replaying A's leaked token is rejected.
	_, err = parseCursorToken(t.Context(), token, storage, filter, "tenant-B")
	require.ErrorIs(t, err, ErrCursorSubjectMismatch)
}

// TestStatefulCursor_UnboundIsBackwardCompatible verifies a token minted without
// a subject stays replayable by any caller — the opt-in default.
func TestStatefulCursor_UnboundIsBackwardCompatible(t *testing.T) {
	t.Parallel()

	storage := newFakeCursorStorage()
	sort := bson.D{{Key: "_id", Value: 1}}
	filter := bson.M{}
	cursorID := bson.NewObjectID().Hex()

	token, err := generateNextCursorToken(t.Context(), cursorID, sort, "_id", filter, storage, nil, "")
	require.NoError(t, err)
	require.Empty(t, storage.entries[token].Subject)

	// Any subject (including empty) may continue the unbound cursor.
	_, err = parseCursorToken(t.Context(), token, storage, filter, "whoever")
	require.NoError(t, err)
	_, err = parseCursorToken(t.Context(), token, storage, filter, "")
	require.NoError(t, err)
}
