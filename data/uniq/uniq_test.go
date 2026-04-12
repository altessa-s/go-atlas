// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package uniq

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdd(t *testing.T) {
	u := NewWithNoop()
	err := u.Add(t.Context(), "key1")
	require.NoError(t, err)
}

func TestAddWithValue(t *testing.T) {
	u := NewWithNoop()
	err := u.AddWithValue(t.Context(), "key1", "value1")
	require.NoError(t, err)
}

func TestGetValue(t *testing.T) {
	u := NewWithNoop()
	// Noop provider returns nil for GetValue, which means ErrDoesNotExist.
	var out string
	err := u.GetValue(t.Context(), "key1", &out)
	require.ErrorIs(t, err, ErrDoesNotExist)
}

func TestExist(t *testing.T) {
	u := NewWithNoop()
	exists, err := u.Exist(t.Context(), "key1")
	require.NoError(t, err)
	require.False(t, exists, "expected false for noop")
}

func TestRemove(t *testing.T) {
	u := NewWithNoop()
	err := u.Remove(t.Context(), "key1")
	require.NoError(t, err)
}

func TestClear(t *testing.T) {
	u := NewWithNoop()
	err := u.Clear(t.Context())
	require.NoError(t, err)
}

func TestAdd_EmptyKey(t *testing.T) {
	u := NewWithNoop()
	err := u.Add(t.Context(), "")
	require.ErrorIs(t, err, ErrInvalidKey)
}

func TestAdd_KeyTooLong(t *testing.T) {
	u := NewWithNoop()
	longKey := strings.Repeat("a", maxKeyLength+1)
	err := u.Add(t.Context(), longKey)
	require.ErrorIs(t, err, ErrInvalidKey)
}

func TestGetValue_NotFound(t *testing.T) {
	u := NewWithNoop()
	var out string
	err := u.GetValue(t.Context(), "missing", &out)
	require.ErrorIs(t, err, ErrDoesNotExist)
}
