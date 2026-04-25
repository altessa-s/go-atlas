// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package noop

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/cache/providers"
)

func TestNoop_Save(t *testing.T) {
	p := New()
	err := p.Save(t.Context(), "key", []byte("value"), time.Minute)
	require.NoError(t, err)
}

func TestNoop_Get_ReturnsErrMissing(t *testing.T) {
	p := New()
	_, err := p.Get(t.Context(), "key")
	require.ErrorIs(t, err, providers.ErrMissing)
}

func TestNoop_Exists_ReturnsFalse(t *testing.T) {
	p := New()
	exists, err := p.Exists(t.Context(), "key")
	require.NoError(t, err)
	require.False(t, exists)
}

func TestNoop_Delete(t *testing.T) {
	p := New()
	err := p.Delete(t.Context(), "key")
	require.NoError(t, err)
}

func TestNoop_DeleteMany(t *testing.T) {
	p := New()
	err := p.DeleteMany(t.Context(), "k1", "k2")
	require.NoError(t, err)
}
