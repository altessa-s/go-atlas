// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package id_test

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/service/id"
)

func TestNewHex(t *testing.T) {
	t.Parallel()

	for _, n := range []int{1, 8, 16, 32} {
		got := id.NewHex(n)
		require.Len(t, got, 2*n)
		require.Equal(t, strings.ToLower(got), got)
		raw, err := hex.DecodeString(got)
		require.NoError(t, err)
		require.Len(t, raw, n)
	}
	require.Empty(t, id.NewHex(0))
	require.Panics(t, func() { id.NewHex(-1) })

	seen := make(map[string]struct{}, 1000)
	for range 1000 {
		v := id.NewHex(16)
		_, dup := seen[v]
		require.False(t, dup, "duplicate 128-bit id %s", v)
		seen[v] = struct{}{}
	}
}
