// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build stress

package memory

import (
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

// bucketOf returns the bucket currently tracked for key, or nil.
func (p *Provider) bucketOf(key string) *bucket {
	sh := p.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	return sh.buckets[key]
}

// admission is one allowed request on a hot key: the bucket that admitted it
// and the wall-clock interval enclosing the Allow call.
type admission struct {
	b          *bucket
	start, end time.Time
}

// overAdmitted reports a set of more than limit admissions by one bucket
// that provably fell inside one period: the last one ended less than period
// after the first one started. Admissions whose bucket was evicted and
// recreated belong to different buckets and are never mixed.
func overAdmitted(adms []admission, limit int, period time.Duration) (admission, admission, bool) {
	byBucket := make(map[*bucket][]admission)
	for _, a := range adms {
		byBucket[a.b] = append(byBucket[a.b], a)
	}
	for _, group := range byBucket {
		slices.SortFunc(group, func(x, y admission) int { return x.start.Compare(y.start) })
		for i := 0; i+limit < len(group); i++ {
			last := group[i]
			for _, a := range group[i+1 : i+limit+1] {
				if a.end.After(last.end) {
					last = a
				}
			}
			if last.end.Sub(group[i].start) < period {
				return group[i], last, true
			}
		}
	}
	return admission{}, admission{}, false
}

// TestStressProvider runs hits on hot keys, a stream of new keys at the cap,
// Reset and RunCleanup together, with one and with eight shards. The bucket
// count must never exceed the cap, the shard structures must stay
// consistent, and no bucket may admit more than its limit within one period.
func TestStressProvider(t *testing.T) {
	duration := testhelpers.StressDuration(t)
	for _, shards := range []int{1, 8} {
		t.Run("shards="+strconv.Itoa(shards), func(t *testing.T) {
			stressProvider(t, shards, duration/2)
		})
	}
}

func stressProvider(t *testing.T, shards int, duration time.Duration) {
	const (
		maxBuckets = 256
		hotKeys    = 8
		limit      = 5
		period     = 20 * time.Millisecond
	)
	p := New(WithMaxBuckets(maxBuckets), WithShards(shards), WithMaxIdleTime(5*time.Millisecond))
	ctx := t.Context()

	var (
		stop    atomic.Bool
		wg      sync.WaitGroup
		admMu   sync.Mutex
		adms    = make([][]admission, hotKeys)
		churned atomic.Int64
	)
	for k := range hotKeys {
		key := "hot-" + strconv.Itoa(k)
		for range 3 {
			wg.Go(func() {
				for !stop.Load() {
					before := p.bucketOf(key)
					start := time.Now()
					_, err := p.Allow(ctx, key, limit, period)
					end := time.Now()
					if err != nil || before == nil || p.bucketOf(key) != before {
						continue // rejected, or the bucket changed: attribution unknown
					}
					admMu.Lock()
					adms[k] = append(adms[k], admission{b: before, start: start, end: end})
					admMu.Unlock()
				}
			})
		}
	}
	for g := range 4 {
		wg.Go(func() {
			for i := 0; !stop.Load(); i++ {
				key := "churn-" + strconv.Itoa(g) + "-" + strconv.Itoa(i)
				_, _ = p.Allow(ctx, key, 1, time.Minute) //nolint:errcheck // only the side effect matters
				churned.Add(1)
				if i%7 == 0 {
					_ = p.Reset(ctx, "churn-"+strconv.Itoa(g)+"-"+strconv.Itoa(i/2)) //nolint:errcheck // always nil
				}
			}
		})
	}
	wg.Go(func() {
		for !stop.Load() {
			p.RunCleanup()
			time.Sleep(time.Millisecond)
		}
	})
	wg.Go(func() {
		for !stop.Load() {
			if n := p.count.Load(); n > maxBuckets || n < 0 {
				t.Errorf("bucket count %d outside [0, %d]", n, maxBuckets)
				stop.Store(true)
			}
		}
	})

	deadline := time.Now().Add(duration)
	checks := 0
	for time.Now().Before(deadline) && !t.Failed() {
		time.Sleep(100 * time.Millisecond)
		require.LessOrEqual(t, p.total(t), maxBuckets, "bucket count exceeds the cap")
		admMu.Lock()
		for k := range adms {
			first, last, bad := overAdmitted(adms[k], limit, period)
			if bad {
				admMu.Unlock()
				require.FailNow(t, "a key admitted more than its limit within one period",
					"hot-%d: %d admissions between %s and %s (period %s)",
					k, limit+1, first.start.Format(time.StampMicro), last.end.Format(time.StampMicro), period)
			}
			// Admissions older than two periods can no longer join a violating set.
			cutoff := time.Now().Add(-2 * period)
			adms[k] = slices.DeleteFunc(adms[k], func(a admission) bool { return a.end.Before(cutoff) })
		}
		admMu.Unlock()
		checks++
	}
	stop.Store(true)
	wg.Wait()
	require.False(t, t.Failed(), "invariant violated during the run")
	require.LessOrEqual(t, p.total(t), maxBuckets)
	t.Logf("shards=%d checks=%d churned keys=%d", shards, checks, churned.Load())
}
