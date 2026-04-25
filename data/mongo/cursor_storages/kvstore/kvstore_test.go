// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kvstore_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/mongo"
	"github.com/altessa-s/go-atlas/data/mongo/cursor_storages/kvstore"
	"github.com/altessa-s/go-atlas/data/mongo/internal/testhelpers"
)

// mockBackend implements kvstore.Backend for testing.
type mockBackend struct {
	store map[string][]byte
	err   error
}

func newMockBackend() *mockBackend {
	return &mockBackend{store: make(map[string][]byte)}
}

func (m *mockBackend) Get(_ context.Context, key string) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	v, ok := m.store[key]
	if !ok {
		return nil, kvstore.ErrKeyNotFound
	}
	return v, nil
}

func (m *mockBackend) Set(_ context.Context, key string, value []byte) error {
	if m.err != nil {
		return m.err
	}
	m.store[key] = value
	return nil
}

func (m *mockBackend) Delete(_ context.Context, key string) error {
	if m.err != nil {
		return m.err
	}
	delete(m.store, key)
	return nil
}

func TestNewJSONStorage(t *testing.T) {
	s := kvstore.NewJSONStorage(newMockBackend(), "test")
	require.NotNil(t, s)
}

func TestJSONStorage_StoreLoad(t *testing.T) {
	b := newMockBackend()
	s := kvstore.NewJSONStorage(b, "test")
	ctx := t.Context()
	meta := testhelpers.SampleCursorMetadata()

	require.NoError(t, s.Store(ctx, "key1", meta))

	loaded, err := s.Load(ctx, "key1")
	require.NoError(t, err)
	require.Equal(t, meta.CursorId, loaded.CursorId)
	require.Equal(t, meta.CursorIdField, loaded.CursorIdField)
}

func TestJSONStorage_Load_NotFound(t *testing.T) {
	b := newMockBackend()
	s := kvstore.NewJSONStorage(b, "test")

	_, err := s.Load(t.Context(), "missing")
	require.ErrorIs(t, err, mongo.ErrCursorNotFound)
}

func TestJSONStorage_Load_BackendError(t *testing.T) {
	b := newMockBackend()
	b.err = errors.New("backend failure")
	s := kvstore.NewJSONStorage(b, "test")

	_, err := s.Load(t.Context(), "key")
	require.Error(t, err)
}

func TestJSONStorage_Store_BackendError(t *testing.T) {
	b := newMockBackend()
	b.err = errors.New("backend failure")
	s := kvstore.NewJSONStorage(b, "test")

	err := s.Store(t.Context(), "key", testhelpers.SampleCursorMetadata())
	require.Error(t, err)
}

func TestJSONStorage_Delete(t *testing.T) {
	b := newMockBackend()
	s := kvstore.NewJSONStorage(b, "test")
	ctx := t.Context()

	_ = s.Store(ctx, "key1", testhelpers.SampleCursorMetadata())
	require.NoError(t, s.Delete(ctx, "key1"))

	_, err := s.Load(ctx, "key1")
	require.ErrorIs(t, err, mongo.ErrCursorNotFound)
}

func TestJSONStorage_Delete_BackendError(t *testing.T) {
	b := newMockBackend()
	b.err = errors.New("backend failure")
	s := kvstore.NewJSONStorage(b, "test")

	err := s.Delete(t.Context(), "key")
	require.Error(t, err)
}

func TestJSONStorage_Load_InvalidJSON(t *testing.T) {
	b := newMockBackend()
	b.store["bad"] = []byte("not-json")
	s := kvstore.NewJSONStorage(b, "test")

	_, err := s.Load(t.Context(), "bad")
	require.Error(t, err)
}
