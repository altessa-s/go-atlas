// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas/core/runtime/panics"
	"github.com/altessa-s/go-atlas/data/limiters/storages"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// bucket represents a sliding window bucket for rate limiting.
type bucket struct {
	// requests holds request timestamps in non-decreasing order (time.Now is
	// monotonic and appends happen under the provider lock). Live entries are
	// requests[head:]; the prefix before head is expired and reclaimed by
	// amortized compaction in Allow.
	requests []time.Time
	head     int
	limit    int64
	window   time.Duration
	lastUsed time.Time
}

// Provider implements an in-memory rate limiting provider using sliding window algorithm.
// Use RunCleanup() to remove expired buckets, either manually or via scheduler.
type Provider struct {
	buckets     map[string]*bucket
	mu          sync.RWMutex
	options     *options
	scheduler   corescheduler.TaskRegistrar
	cleanupTask corescheduler.ManagedTask // Guards RunCleanup and marks scheduler management.
}

// New creates a new memory-based rate limiting provider.
//
// If scheduler is provided and cleanupSchedule is set, cleanup task will be registered.
// Otherwise, call RunCleanup() manually or register it externally.
func New(opt ...Option) *Provider {
	opts := newOptions(opt...)
	p := &Provider{
		options:   opts,
		buckets:   make(map[string]*bucket),
		scheduler: opts.scheduler,
	}

	// Register cleanup task with scheduler if provided
	_ = p.registerCleanupTask(opts.cleanupSchedule) //nolint:errcheck // task registration is optional

	return p
}

// Allow checks if a request should be allowed based on the rate limit using sliding window algorithm.
func (p *Provider) Allow(ctx context.Context, key string, limit int64, period time.Duration) (*storages.LimitInfo, error) {
	panics.MustNonNil(ctx, "context must not be nil")
	panics.Must(key != "", "key must not be empty")
	panics.Must(limit > 0, "limit must be greater than 0")
	panics.Must(period > 0, "period must be greater than 0")

	now := time.Now()

	p.mu.Lock()
	defer p.mu.Unlock()
	b, exists := p.buckets[key]

	if !exists {
		b = &bucket{
			requests: make([]time.Time, 0, 16), //nolint:mnd // reasonable initial capacity for sliding window
			limit:    limit,
			window:   period,
			lastUsed: now,
		}
	}

	b.lastUsed = now
	b.limit = limit
	b.window = period

	cutoff := now.Add(-period)

	// Timestamps are sorted (see bucket.requests), so the expired prefix ends
	// at a binary-searchable index — O(log n) instead of rescanning the whole
	// window on every call.
	live := b.requests[b.head:]
	b.head += sort.Search(len(live), func(i int) bool { return live[i].After(cutoff) })

	// Compact once the expired prefix dominates, keeping the backing array
	// bounded at ~2x the live window with amortized O(1) cost per call.
	if b.head > len(b.requests)/2 {
		kept := copy(b.requests, b.requests[b.head:])
		b.requests = b.requests[:kept]
		b.head = 0
	}
	validRequests := len(b.requests) - b.head

	resetTime := uint64(0)
	if validRequests > 0 {
		resetTime = uint64(b.requests[b.head].Add(period).Unix()) // #nosec G115 -- Unix timestamps are positive
	}

	// Check if rate limit is exceeded
	if int64(validRequests) >= limit {
		// #nosec G115 -- resetTime is Unix seconds, fits int64
		return &storages.LimitInfo{Remaining: 0, Reset: int64(resetTime)}, storages.ErrLimitExceeded
	}

	// Add the bucket if it doesn't exist yet. When the configured cap is
	// reached, evict the least-recently-used bucket to make room — this
	// bounds memory hard even when no cleanup scheduler is wired up,
	// closing the DoS window an attacker driving arbitrary keys could
	// otherwise exploit.
	if !exists {
		if p.options.maxBuckets > 0 && len(p.buckets) >= p.options.maxBuckets {
			p.evictLRULocked()
		}
		p.buckets[key] = b
	}

	// Record the new request
	b.requests = append(b.requests, now)

	// Calculate remaining after adding this request
	info := &storages.LimitInfo{
		Remaining: max(limit-int64(len(b.requests)-b.head), 0),
		Reset:     int64(resetTime), // #nosec G115 -- resetTime is Unix seconds, fits int64
	}

	return info, nil
}

// Reset resets the rate limit for a specific key.
func (p *Provider) Reset(ctx context.Context, key string) error {
	panics.MustNonNil(ctx, "context must not be nil")
	panics.Must(key != "", "key must not be empty")

	p.mu.Lock()
	defer p.mu.Unlock()

	if b, exists := p.buckets[key]; exists {
		b.requests = b.requests[:0]
		b.head = 0
	}

	return nil
}

// Close releases any resources held by the provider.
// No-op since there are no background goroutines to stop.
func (p *Provider) Close() error {
	return nil
}

// evictLRULocked drops the bucket with the oldest lastUsed timestamp.
// The caller MUST hold p.mu in write mode. A full O(n) scan is acceptable
// because this only runs at the cap boundary (rarely) and the cap itself
// keeps n small enough (default 100k) that the scan completes in low
// milliseconds. The map is not ordered, so we cannot do better without
// a parallel LRU list — accepting the linear scan in exchange for keeping
// the hot Allow path lock-free of list maintenance.
func (p *Provider) evictLRULocked() {
	var (
		oldestKey  string
		oldestTime time.Time
		first      = true
	)
	for k, v := range p.buckets {
		if first || v.lastUsed.Before(oldestTime) {
			oldestKey = k
			oldestTime = v.lastUsed
			first = false
		}
	}
	if !first {
		delete(p.buckets, oldestKey)
	}
}

var _ storages.Storage = (*Provider)(nil)
