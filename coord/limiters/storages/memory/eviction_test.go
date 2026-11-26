// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/limiters"
	"github.com/altessa-s/go-atlas/coord/limiters/storages/memory"
)

func TestProvider_DefaultEvictionPreservesLiveWindow(t *testing.T) {
	t.Parallel()
	p := memory.New(memory.WithMaxBuckets(3))
	_, err := p.Allow(t.Context(), "victim", 1, time.Hour)
	require.NoError(t, err)
	for _, key := range []string{"one", "two", "three"} {
		_, err = p.Allow(t.Context(), key, 1, time.Hour)
		if key == "three" {
			require.ErrorIs(t, err, memory.ErrCapacityExceeded)
		} else {
			require.NoError(t, err)
		}
	}
	_, err = p.Allow(t.Context(), "victim", 1, time.Hour)
	require.ErrorIs(t, err, limiters.ErrLimitExceeded)
}
