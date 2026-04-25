// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package probfilter_test

import (
	"context"
	"errors"
	"io"
	"iter"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/probfilter"
)

type mockFilter struct {
	closeErr error
}

func (m *mockFilter) MightExist(_ context.Context, _ string) (bool, error) { return false, nil }
func (m *mockFilter) Add(_ context.Context, _ string) error                { return nil }
func (m *mockFilter) AddBatch(_ context.Context, _ iter.Seq[string]) error { return nil }
func (m *mockFilter) Close() error                                         { return m.closeErr }

var _ io.Closer = (*mockFilter)(nil)

func TestManager_Register_Duplicate(t *testing.T) {
	mgr := probfilter.NewManager()
	f := &mockFilter{}

	require.NoError(t, mgr.Register("test", f))

	err := mgr.Register("test", f)
	require.ErrorIs(t, err, probfilter.ErrFilterAlreadyExists)
}

func TestManager_Get_NotFound(t *testing.T) {
	mgr := probfilter.NewManager()

	_, err := mgr.Get("nonexistent")
	require.ErrorIs(t, err, probfilter.ErrFilterNotFound)
}

func TestManager_MustGet_Panics(t *testing.T) {
	mgr := probfilter.NewManager()

	defer func() {
		if r := recover(); r == nil {
			t.Error("MustGet should panic for missing filter")
		}
	}()

	mgr.MustGet("nonexistent")
}

func TestManager_MustGet_Success(t *testing.T) {
	mgr := probfilter.NewManager()
	f := &mockFilter{}
	_ = mgr.Register("test", f)

	got := mgr.MustGet("test")
	require.Equal(t, f, got)
}

func TestManager_Unregister(t *testing.T) {
	mgr := probfilter.NewManager()
	f := &mockFilter{}
	_ = mgr.Register("test", f)

	mgr.Unregister("test")

	_, err := mgr.Get("test")
	require.ErrorIs(t, err, probfilter.ErrFilterNotFound)
}

func TestManager_Unregister_Nonexistent(t *testing.T) {
	mgr := probfilter.NewManager()
	// Should not panic
	mgr.Unregister("nonexistent")
}

func TestManager_Names(t *testing.T) {
	mgr := probfilter.NewManager()
	_ = mgr.Register("a", &mockFilter{})
	_ = mgr.Register("b", &mockFilter{})

	names := make(map[string]bool)
	for name := range mgr.Names() {
		names[name] = true
	}

	require.True(t, names["a"], "Names() missing 'a'")
	require.True(t, names["b"], "Names() missing 'b'")
}

func TestManager_Names_Empty(t *testing.T) {
	mgr := probfilter.NewManager()
	count := 0
	for range mgr.Names() {
		count++
	}
	require.Equal(t, 0, count)
}

func TestManager_Filters(t *testing.T) {
	mgr := probfilter.NewManager()
	f1 := &mockFilter{}
	f2 := &mockFilter{}
	_ = mgr.Register("a", f1)
	_ = mgr.Register("b", f2)

	filters := make(map[string]probfilter.Filter)
	for name, filter := range mgr.Filters() {
		filters[name] = filter
	}

	require.Equal(t, f1, filters["a"])
	require.Equal(t, f2, filters["b"])
}

func TestManager_Close(t *testing.T) {
	mgr := probfilter.NewManager()
	_ = mgr.Register("ok", &mockFilter{})

	err := mgr.Close()
	require.NoError(t, err)

	// After close, filters should be cleared
	count := 0
	for range mgr.Names() {
		count++
	}
	require.Equal(t, 0, count)
}

func TestManager_Close_WithErrors(t *testing.T) {
	mgr := probfilter.NewManager()
	_ = mgr.Register("fail1", &mockFilter{closeErr: errors.New("err1")})
	_ = mgr.Register("fail2", &mockFilter{closeErr: errors.New("err2")})

	err := mgr.Close()
	require.Error(t, err, "Close() should return error when filters fail to close")
}

func TestManager_Close_Empty(t *testing.T) {
	mgr := probfilter.NewManager()
	require.NoError(t, mgr.Close())
}
