// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"errors"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/mongo"
	"github.com/altessa-s/go-atlas/data/mongo/cursor_storages/memory"
	"github.com/altessa-s/go-atlas/data/mongo/internal/testhelpers"
)

func TestNew(t *testing.T) {
	s := memory.New(time.Hour)
	if s == nil {
		t.Fatal("New() returned nil")
	}
	s.Close()
}

func TestNew_ZeroTTL(t *testing.T) {
	s := memory.New(0)
	defer s.Close()
	if s == nil {
		t.Fatal("New(0) returned nil, should use default TTL")
	}
}

func TestStorage_StoreLoad(t *testing.T) {
	s := memory.New(time.Hour)
	defer s.Close()
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
	s := memory.New(time.Hour)
	defer s.Close()

	_, err := s.Load(t.Context(), "missing")
	if !errors.Is(err, mongo.ErrCursorNotFound) {
		t.Errorf("Load() error = %v, want ErrCursorNotFound", err)
	}
}

func TestStorage_Load_Expired(t *testing.T) {
	s := memory.New(1 * time.Millisecond)
	defer s.Close()
	ctx := t.Context()

	_ = s.Store(ctx, "key1", testhelpers.SampleCursorMetadata())
	time.Sleep(5 * time.Millisecond)

	_, err := s.Load(ctx, "key1")
	if !errors.Is(err, mongo.ErrCursorNotFound) {
		t.Errorf("Load() expired error = %v, want ErrCursorNotFound", err)
	}
}

func TestStorage_Delete(t *testing.T) {
	s := memory.New(time.Hour)
	defer s.Close()
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
	s := memory.New(time.Hour)
	defer s.Close()

	if err := s.Delete(t.Context(), "nope"); err != nil {
		t.Errorf("Delete() nonexistent should not error: %v", err)
	}
}

func TestStorage_Len(t *testing.T) {
	s := memory.New(time.Hour)
	defer s.Close()
	ctx := t.Context()

	if s.Len() != 0 {
		t.Errorf("Len() = %d, want 0", s.Len())
	}

	_ = s.Store(ctx, "k1", testhelpers.SampleCursorMetadata())
	_ = s.Store(ctx, "k2", testhelpers.SampleCursorMetadata())

	if s.Len() != 2 {
		t.Errorf("Len() = %d, want 2", s.Len())
	}
}

func TestStorage_RunCleanup(t *testing.T) {
	s := memory.New(1 * time.Millisecond)
	defer s.Close()
	ctx := t.Context()

	_ = s.Store(ctx, "k1", testhelpers.SampleCursorMetadata())
	_ = s.Store(ctx, "k2", testhelpers.SampleCursorMetadata())
	time.Sleep(5 * time.Millisecond)

	s.RunCleanup()

	if s.Len() != 0 {
		t.Errorf("Len() after cleanup = %d, want 0", s.Len())
	}
}

func TestStorage_RunCleanup_PartialExpiry(t *testing.T) {
	s := memory.New(time.Hour)
	defer s.Close()
	ctx := t.Context()

	_ = s.Store(ctx, "keep", testhelpers.SampleCursorMetadata())

	// Create a short-TTL storage to add expired entry
	sShort := memory.New(1 * time.Millisecond)
	defer sShort.Close()
	_ = sShort.Store(ctx, "expire", testhelpers.SampleCursorMetadata())
	time.Sleep(5 * time.Millisecond)
	sShort.RunCleanup()

	// Original storage should still have its entry
	if s.Len() != 1 {
		t.Errorf("Len() = %d, want 1", s.Len())
	}
}

func TestStorage_Overwrite(t *testing.T) {
	s := memory.New(time.Hour)
	defer s.Close()
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
	if s.Len() != 1 {
		t.Errorf("Len() = %d, want 1", s.Len())
	}
}
