// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package generation_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/cache/generation"
)

func TestSeed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		now  time.Time
		want uint64
	}{
		{name: "before the epoch clamps to zero", now: time.Unix(-1, 0), want: 0},
		{name: "the epoch", now: time.Unix(0, 0), want: 0},
		{name: "after the epoch in microseconds", now: time.Unix(1_700_000_000, 123_456_789), want: 1_700_000_000_123_456},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, generation.Seed(tc.now))
		})
	}
}
