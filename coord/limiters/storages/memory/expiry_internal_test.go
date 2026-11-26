// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/time/clock"
)

func TestProvider_AtCapacityVisitsBoundedBuckets(t *testing.T) {
	t.Parallel()
	for _, capacity := range []int{100, 10_000} {
		t.Run(strconv.Itoa(capacity), func(t *testing.T) {
			t.Parallel()
			fake := clock.NewFake(time.Now())
			p := New(WithClock(fake), WithMaxBuckets(capacity), WithShards(4))
			visits := 0
			p.visitReclaimBucket = func() { visits++ }
			for i := range capacity {
				_, err := p.Allow(t.Context(), strconv.Itoa(i), 1, time.Second)
				require.NoError(t, err)
				fake.Advance(time.Millisecond)
			}
			// The first window is exactly one period old at each admission.
			// For a large capacity there may also be a backlog of expired windows.
			if capacity < 1000 {
				fake.Advance(time.Second - time.Duration(capacity)*time.Millisecond)
			}
			for i := range 20 {
				visits = 0
				_, err := p.Allow(t.Context(), "new-"+strconv.Itoa(i), 1, time.Second)
				require.NoError(t, err)
				require.Positive(t, visits)
				require.LessOrEqual(t, visits, len(p.shards)+1)
				fake.Advance(time.Millisecond)
			}
		})
	}
}

func BenchmarkProvider_Allow_ReclaimAtCapacity(b *testing.B) {
	for _, capacity := range []int{1_000, 100_000} {
		b.Run(strconv.Itoa(capacity), func(b *testing.B) {
			fake := clock.NewFake(time.Now())
			p := New(WithClock(fake), WithMaxBuckets(capacity), WithShards(4))
			const step = time.Millisecond
			period := time.Duration(capacity) * step
			for i := range capacity {
				_, err := p.Allow(b.Context(), strconv.Itoa(i), 1, period)
				require.NoError(b, err)
				fake.Advance(step)
			}
			i := 0
			b.ReportAllocs()
			for b.Loop() {
				_, err := p.Allow(b.Context(), "new-"+strconv.Itoa(i), 1, period)
				require.NoError(b, err)
				fake.Advance(step)
				i++
			}
		})
	}
}
