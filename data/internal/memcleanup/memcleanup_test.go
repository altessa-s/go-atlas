// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memcleanup_test

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/internal/memcleanup"
)

type item struct {
	expiresAt time.Time
}

func TestSweep(t *testing.T) {
	t.Parallel()

	now := time.Now()
	past := now.Add(-time.Minute)
	future := now.Add(time.Minute)

	tests := []struct {
		name     string
		entries  map[string]*item
		wantKeys []string
	}{
		{
			name:     "empty map is a no-op",
			entries:  map[string]*item{},
			wantKeys: []string{},
		},
		{
			name: "nothing expired keeps all entries",
			entries: map[string]*item{
				"a": {expiresAt: future},
				"b": {expiresAt: future},
			},
			wantKeys: []string{"a", "b"},
		},
		{
			name: "expired entries are removed, live ones kept",
			entries: map[string]*item{
				"expired-1": {expiresAt: past},
				"expired-2": {expiresAt: past},
				"live":      {expiresAt: future},
			},
			wantKeys: []string{"live"},
		},
		{
			name: "all expired empties the map",
			entries: map[string]*item{
				"a": {expiresAt: past},
				"b": {expiresAt: past},
			},
			wantKeys: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var mu sync.RWMutex
			memcleanup.Sweep(&mu, tc.entries, func(it *item) bool {
				return now.After(it.expiresAt)
			})

			require.Len(t, tc.entries, len(tc.wantKeys))
			for _, k := range tc.wantKeys {
				require.Contains(t, tc.entries, k)
			}
		})
	}
}

// TestSweep_RefreshedBetweenPhasesSurvives pins the phase-2 re-check that
// fixes the premature-delete race in the idempotency memory storage: an entry
// that is expired when phase 1 collects it but whose TTL is re-armed before
// phase 2 deletes must survive the sweep.
//
// The predicate simulates the concurrent refresh deterministically: its
// phase-1 evaluation of the target entry reports "expired" (so the key is
// collected) and re-arms the expiry, exactly as a writer squeezing between
// the sweep's RUnlock and Lock would; the phase-2 re-evaluation then sees the
// fresh expiry and must spare the entry.
func TestSweep_RefreshedBetweenPhasesSurvives(t *testing.T) {
	t.Parallel()

	now := time.Now()
	past := now.Add(-time.Minute)
	future := now.Add(time.Minute)

	refreshed := &item{expiresAt: past}
	entries := map[string]*item{
		"refreshed": refreshed,
		"expired":   {expiresAt: past},
		"live":      {expiresAt: future},
	}

	var mu sync.RWMutex
	memcleanup.Sweep(&mu, entries, func(it *item) bool {
		expired := now.After(it.expiresAt)
		if it == refreshed && expired {
			it.expiresAt = future // Re-arm the TTL after the phase-1 verdict.
		}
		return expired
	})

	require.Contains(t, entries, "refreshed", "entry refreshed between phases must survive the sweep")
	require.Contains(t, entries, "live")
	require.NotContains(t, entries, "expired")
}

// TestSweep_ConcurrentWriters exercises the sweep against writers that
// insert, refresh, and delete entries under the same lock. Run with -race.
func TestSweep_ConcurrentWriters(t *testing.T) {
	t.Parallel()

	var mu sync.RWMutex
	entries := make(map[int]*item)
	base := time.Now()
	for i := range 512 {
		entries[i] = &item{expiresAt: base.Add(time.Duration(i%2) * time.Hour)}
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Go(func() {
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			mu.Lock()
			key := i % 512
			if i%3 == 0 {
				delete(entries, key)
			} else {
				entries[key] = &item{expiresAt: time.Now().Add(time.Hour)}
			}
			mu.Unlock()
		}
	})

	for range 50 {
		now := time.Now()
		memcleanup.Sweep(&mu, entries, func(it *item) bool {
			return now.After(it.expiresAt)
		})
	}
	close(stop)
	wg.Wait()

	// Every surviving entry must be unexpired: writers only ever insert
	// fresh entries, and the sweep may not remove them.
	mu.RLock()
	defer mu.RUnlock()
	for k, it := range entries {
		require.True(t, it.expiresAt.After(base), "entry %d must be live", k)
	}
}
