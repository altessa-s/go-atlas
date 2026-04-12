// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package noop_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/uniq/providers/noop"
)

func TestNew(t *testing.T) {
	p := noop.New()
	require.NotNil(t, p, "New() returned nil")
}

func TestProvider_Add(t *testing.T) {
	p := noop.New()
	require.NoError(t, p.Add(t.Context(), "key"))
}

func TestProvider_AddWithValue(t *testing.T) {
	p := noop.New()
	require.NoError(t, p.AddWithValue(t.Context(), "key", []byte("val")))
}

func TestProvider_Exist(t *testing.T) {
	p := noop.New()
	exists, err := p.Exist(t.Context(), "key")
	require.NoError(t, err)
	require.False(t, exists, "Exist() should return false for noop")
}

func TestProvider_GetValue(t *testing.T) {
	p := noop.New()
	val, err := p.GetValue(t.Context(), "key")
	require.NoError(t, err)
	require.Nil(t, val)
}

func TestProvider_Remove(t *testing.T) {
	p := noop.New()
	require.NoError(t, p.Remove(t.Context(), "key"))
}

func TestProvider_Clear(t *testing.T) {
	p := noop.New()
	require.NoError(t, p.Clear(t.Context()))
}

func TestProvider_FullLifecycle(t *testing.T) {
	p := noop.New()
	ctx := t.Context()

	_ = p.Add(ctx, "key1")
	_ = p.AddWithValue(ctx, "key2", []byte("data"))

	exists, _ := p.Exist(ctx, "key1")
	require.False(t, exists, "noop Exist() should always be false")

	val, _ := p.GetValue(ctx, "key2")
	require.Nil(t, val)

	_ = p.Remove(ctx, "key1")
	_ = p.Clear(ctx)
}
