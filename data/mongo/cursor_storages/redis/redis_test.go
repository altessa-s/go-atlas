// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/altessa-s/go-atlas/data/mongo"
	"github.com/altessa-s/go-atlas/data/mongo/internal/testhelpers"

	cursredis "github.com/altessa-s/go-atlas/data/mongo/cursor_storages/redis"
	goredis "github.com/redis/go-redis/v9"
)

func setupStorage(tb testing.TB) *cursredis.Storage {
	tb.Helper()
	mr := miniredis.RunT(tb)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	tb.Cleanup(func() { client.Close() })
	return cursredis.New(client)
}

func TestNew(t *testing.T) {
	s := setupStorage(t)
	if s == nil {
		t.Fatal("New() returned nil")
	}
}

func TestStorage_StoreLoad(t *testing.T) {
	s := setupStorage(t)
	ctx := t.Context()
	meta := testhelpers.SampleCursorMetadata()

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
}

func TestStorage_Load_NotFound(t *testing.T) {
	s := setupStorage(t)

	_, err := s.Load(t.Context(), "missing")
	if !errors.Is(err, mongo.ErrCursorNotFound) {
		t.Errorf("Load() error = %v, want ErrCursorNotFound", err)
	}
}

func TestStorage_Delete(t *testing.T) {
	s := setupStorage(t)
	ctx := t.Context()

	_ = s.Store(ctx, "key1", testhelpers.SampleCursorMetadata())
	if err := s.Delete(ctx, "key1"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	_, err := s.Load(ctx, "key1")
	if !errors.Is(err, mongo.ErrCursorNotFound) {
		t.Errorf("Load() after Delete() error = %v, want ErrCursorNotFound", err)
	}
}

func TestStorage_Delete_NonExistent(t *testing.T) {
	s := setupStorage(t)
	if err := s.Delete(t.Context(), "nope"); err != nil {
		t.Errorf("Delete() nonexistent should not error: %v", err)
	}
}

func TestStorage_Overwrite(t *testing.T) {
	s := setupStorage(t)
	ctx := t.Context()

	meta1 := testhelpers.SampleCursorMetadata()
	meta1.CursorId = "aaa"
	_ = s.Store(ctx, "key", meta1)

	meta2 := testhelpers.SampleCursorMetadata()
	meta2.CursorId = "bbb"
	_ = s.Store(ctx, "key", meta2)

	loaded, _ := s.Load(ctx, "key")
	if loaded.CursorId != "bbb" {
		t.Errorf("CursorId = %q, want %q", loaded.CursorId, "bbb")
	}
}

func TestStorage_WithCustomOptions(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })

	s := cursredis.New(client,
		cursredis.WithTtl(30*time.Minute),
		cursredis.WithKeyPrefix("custom:"),
	)

	ctx := t.Context()
	_ = s.Store(ctx, "key1", testhelpers.SampleCursorMetadata())

	loaded, err := s.Load(ctx, "key1")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if loaded.CursorId != testhelpers.SampleCursorMetadata().CursorId {
		t.Errorf("CursorId mismatch")
	}
}
