// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

// White-box: a failed insert must leave the bucket table byte-for-byte
// unchanged, which needs access to the private table.
func TestCuckooFilter_FailedInsertLeavesTableUnchanged(t *testing.T) {
	t.Parallel()
	f := newCuckooFilter(64)

	failures := 0
	for i := 0; failures < 50; i++ {
		before := slices.Clone(f.buckets)
		count := f.count
		if !f.insert(fmt.Sprintf("v-%d", i)) {
			failures++
			require.Equal(t, before, f.buckets, "failed insert %d changed the table", i)
			require.Equal(t, count, f.count)
		}
	}
}

func TestCuckooFilter_Sizing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		capacity  uint
		wantSlots uint
	}{
		{capacity: 0, wantSlots: 4},
		{capacity: 1, wantSlots: 4},
		{capacity: 4, wantSlots: 4},
		{capacity: 5, wantSlots: 8},
		{capacity: 1000, wantSlots: 1024},
		{capacity: 1024, wantSlots: 1024},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprint(tc.capacity), func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.wantSlots, newCuckooFilter(tc.capacity).slots())
		})
	}
}

func TestCuckooFilter_AltIsInvolution(t *testing.T) {
	t.Parallel()
	f := newCuckooFilter(1 << 12)
	for i := range uint64(len(f.buckets)) {
		for fp := fingerprint(1); fp != 0; fp++ {
			require.Equal(t, i, f.alt(f.alt(i, fp), fp))
		}
	}
}
