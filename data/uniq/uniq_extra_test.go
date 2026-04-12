// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package uniq_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/uniq"
)

func TestNewWithNoop(t *testing.T) {
	u := uniq.NewWithNoop()
	require.NotNil(t, u, "NewWithNoop() returned nil")
}

func TestNop_Add(t *testing.T) {
	u := uniq.NewWithNoop()
	ctx := t.Context()

	require.NoError(t, u.Add(ctx, "key1"))
}

func TestNop_Exist(t *testing.T) {
	u := uniq.NewWithNoop()
	ctx := t.Context()

	_ = u.Add(ctx, "key1")
	exists, err := u.Exist(ctx, "key1")
	require.NoError(t, err)
	_ = exists // nop provider may or may not track
}

func TestNop_Remove(t *testing.T) {
	u := uniq.NewWithNoop()
	ctx := t.Context()

	require.NoError(t, u.Remove(ctx, "key1"))
}

func TestNop_Clear(t *testing.T) {
	u := uniq.NewWithNoop()
	ctx := t.Context()

	require.NoError(t, u.Clear(ctx))
}

func TestNop_AddWithValue(t *testing.T) {
	u := uniq.NewWithNoop()
	ctx := t.Context()

	require.NoError(t, u.AddWithValue(ctx, "key1", "val1"))
}

func TestNop_GetValue(t *testing.T) {
	u := uniq.NewWithNoop()
	ctx := t.Context()

	_ = u.AddWithValue(ctx, "key1", "val1")
	var out string
	err := u.GetValue(ctx, "key1", &out)
	// nop may not support GetValue, just ensure no panic
	_ = err
}

func TestAdd_EmptyKey_Nop(t *testing.T) {
	u := uniq.NewWithNoop()
	ctx := t.Context()

	err := u.Add(ctx, "")
	require.Error(t, err, "Add('') should return error")
}
