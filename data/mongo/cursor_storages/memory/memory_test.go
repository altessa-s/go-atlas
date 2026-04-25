// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/mongo"
	"github.com/altessa-s/go-atlas/data/mongo/cursor_storages/memory"
	"github.com/altessa-s/go-atlas/data/mongo/internal/testhelpers"
)

func TestNew(t *testing.T) {
	s := memory.New(time.Hour)
	require.NotNil(t, s)
	s.Close()
}

func TestNew_ZeroTTL(t *testing.T) {
	s := memory.New(0)
	defer s.Close()
	require.NotNil(t, s)
}

func TestStorage_StoreLoad(t *testing.T) {
	s := memory.New(time.Hour)
	defer s.Close()
	ctx := t.Context()
	meta := testhelpers.SampleCursorMetadata()

	require.NoError(t, s.Store(ctx, "key1", meta))

	loaded, err := s.Load(ctx, "key1")
	require.NoError(t, err)
	require.Equal(t, meta.CursorId, loaded.CursorId)
}

func TestStorage_Load_NotFound(t *testing.T) {
	s := memory.New(time.Hour)
	defer s.Close()

	_, err := s.Load(t.Context(), "missing")
	require.ErrorIs(t, err, mongo.ErrCursorNotFound)
}

func TestStorage_Load_Expired(t *testing.T) {
	s := memory.New(1 * time.Millisecond)
	defer s.Close()
	ctx := t.Context()

	_ = s.Store(ctx, "key1", testhelpers.SampleCursorMetadata())
	time.Sleep(5 * time.Millisecond)

	_, err := s.Load(ctx, "key1")
	require.ErrorIs(t, err, mongo.ErrCursorNotFound)
}

func TestStorage_Delete(t *testing.T) {
	s := memory.New(time.Hour)
	defer s.Close()
	ctx := t.Context()

	_ = s.Store(ctx, "key1", testhelpers.SampleCursorMetadata())
	require.NoError(t, s.Delete(ctx, "key1"))

	_, err := s.Load(ctx, "key1")
	require.ErrorIs(t, err, mongo.ErrCursorNotFound)
}

func TestStorage_Delete_NonExistent(t *testing.T) {
	s := memory.New(time.Hour)
	defer s.Close()

	require.NoError(t, s.Delete(t.Context(), "nope"))
}

func TestStorage_Len(t *testing.T) {
	s := memory.New(time.Hour)
	defer s.Close()
	ctx := t.Context()

	require.Equal(t, 0, s.Len())

	_ = s.Store(ctx, "k1", testhelpers.SampleCursorMetadata())
	_ = s.Store(ctx, "k2", testhelpers.SampleCursorMetadata())

	require.Equal(t, 2, s.Len())
}

func TestStorage_RunCleanup(t *testing.T) {
	s := memory.New(1 * time.Millisecond)
	defer s.Close()
	ctx := t.Context()

	_ = s.Store(ctx, "k1", testhelpers.SampleCursorMetadata())
	_ = s.Store(ctx, "k2", testhelpers.SampleCursorMetadata())
	time.Sleep(5 * time.Millisecond)

	s.RunCleanup()

	require.Equal(t, 0, s.Len())
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
	require.Equal(t, 1, s.Len())
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
	require.Equal(t, "bbb", loaded.CursorId)
	require.Equal(t, 1, s.Len())
}
