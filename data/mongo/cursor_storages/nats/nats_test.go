// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/mongo"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	cursnats "github.com/altessa-s/go-atlas/data/mongo/cursor_storages/nats"
	mongohelpers "github.com/altessa-s/go-atlas/data/mongo/internal/testhelpers"
)

func setupStorage(tb testing.TB) *cursnats.Storage {
	tb.Helper()
	ns := testhelpers.StartNATSServer(tb)
	_, js := testhelpers.ConnectJetStream(tb, ns)

	bucket := strings.ReplaceAll(tb.Name(), "/", "-")
	kv := testhelpers.CreateNATSKV(tb, js, bucket, time.Hour)
	return cursnats.New(kv)
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
	meta := mongohelpers.SampleCursorMetadata()

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

	_ = s.Store(ctx, "key1", mongohelpers.SampleCursorMetadata())
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

	meta1 := mongohelpers.SampleCursorMetadata()
	meta1.CursorId = "aaa"
	_ = s.Store(ctx, "key", meta1)

	meta2 := mongohelpers.SampleCursorMetadata()
	meta2.CursorId = "bbb"
	_ = s.Store(ctx, "key", meta2)

	loaded, _ := s.Load(ctx, "key")
	if loaded.CursorId != "bbb" {
		t.Errorf("CursorId = %q, want %q", loaded.CursorId, "bbb")
	}
}

func TestStorage_Ping(t *testing.T) {
	s := setupStorage(t)
	if err := s.Ping(t.Context()); err != nil {
		t.Errorf("Ping() error: %v", err)
	}
}
