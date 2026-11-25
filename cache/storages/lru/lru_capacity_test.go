// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lru_test

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/cache/storages/lru"

	cachelru "github.com/altessa-s/go-atlas/cache/lru"
)

func TestGetMany_SmallCacheCapacity(t *testing.T) {
	t.Parallel()

	const capacity = 100
	s, err := lru.New(capacity, cachelru.WithShardCount(16))
	require.NoError(t, err)

	keys := make([]string, capacity)
	for i := range keys {
		keys[i] = strconv.Itoa(i)
		require.NoError(t, s.Save(t.Context(), keys[i], []byte(keys[i]), 0))
	}

	values, err := s.GetMany(t.Context(), keys)
	require.NoError(t, err)
	require.Len(t, values, capacity)
}
