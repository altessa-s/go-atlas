// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kvstore_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/mongo"
	"github.com/altessa-s/go-atlas/data/mongo/cursor_storages/kvstore"
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

func sampleMetadata() *mongo.CursorMetadata {
	return &mongo.CursorMetadata{
		CursorId:      "507f1f77bcf86cd799439011",
		Sort:          "dGVzdA==",
		CursorIdField: "_id",
		FilterHash:    "abc123",
		CreatedAt:     time.Now(),
	}
}

func TestNewJSONStorage(t *testing.T) {
	s := kvstore.NewJSONStorage(newMockBackend(), "test")
	if s == nil {
		t.Fatal("NewJSONStorage() returned nil")
	}
}

func TestJSONStorage_StoreLoad(t *testing.T) {
	b := newMockBackend()
	s := kvstore.NewJSONStorage(b, "test")
	ctx := t.Context()
	meta := sampleMetadata()

	if err := s.Store(ctx, "key1", meta); err != nil {
		t.Fatalf("Store() error: %v", err)
	}

	loaded, err := s.Load(ctx, "key1")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if loaded.CursorId != meta.CursorId {
		t.Errorf("CursorId = %q, want %q", loaded.CursorId, meta.CursorId)
	}
	if loaded.CursorIdField != meta.CursorIdField {
		t.Errorf("CursorIdField = %q, want %q", loaded.CursorIdField, meta.CursorIdField)
	}
}

func TestJSONStorage_Load_NotFound(t *testing.T) {
	b := newMockBackend()
	s := kvstore.NewJSONStorage(b, "test")

	_, err := s.Load(t.Context(), "missing")
	if !errors.Is(err, mongo.ErrCursorNotFound) {
		t.Errorf("Load() error = %v, want ErrCursorNotFound", err)
	}
}

func TestJSONStorage_Load_BackendError(t *testing.T) {
	b := newMockBackend()
	b.err = errors.New("backend failure")
	s := kvstore.NewJSONStorage(b, "test")

	_, err := s.Load(t.Context(), "key")
	if err == nil {
		t.Error("Load() should return error on backend failure")
	}
}

func TestJSONStorage_Store_BackendError(t *testing.T) {
	b := newMockBackend()
	b.err = errors.New("backend failure")
	s := kvstore.NewJSONStorage(b, "test")

	err := s.Store(t.Context(), "key", sampleMetadata())
	if err == nil {
		t.Error("Store() should return error on backend failure")
	}
}

func TestJSONStorage_Delete(t *testing.T) {
	b := newMockBackend()
	s := kvstore.NewJSONStorage(b, "test")
	ctx := t.Context()

	_ = s.Store(ctx, "key1", sampleMetadata())
	if err := s.Delete(ctx, "key1"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	_, err := s.Load(ctx, "key1")
	if !errors.Is(err, mongo.ErrCursorNotFound) {
		t.Errorf("Load() after Delete() error = %v, want ErrCursorNotFound", err)
	}
}

func TestJSONStorage_Delete_BackendError(t *testing.T) {
	b := newMockBackend()
	b.err = errors.New("backend failure")
	s := kvstore.NewJSONStorage(b, "test")

	err := s.Delete(t.Context(), "key")
	if err == nil {
		t.Error("Delete() should return error on backend failure")
	}
}

func TestJSONStorage_Load_InvalidJSON(t *testing.T) {
	b := newMockBackend()
	b.store["bad"] = []byte("not-json")
	s := kvstore.NewJSONStorage(b, "test")

	_, err := s.Load(t.Context(), "bad")
	if err == nil {
		t.Error("Load() should return error for invalid JSON")
	}
}
