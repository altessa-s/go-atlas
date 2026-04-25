// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/altessa-s/go-atlas/core/runtime/panics"
	"github.com/altessa-s/go-atlas/data/limiters/storages"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// bucket represents a sliding window bucket for rate limiting.
type bucket struct {
	requests []time.Time
	limit    int64
	window   time.Duration
	lastUsed time.Time
}

// Provider implements an in-memory rate limiting provider using sliding window algorithm.
// Use RunCleanup() to remove expired buckets, either manually or via scheduler.
type Provider struct {
	buckets        map[string]*bucket
	mu             sync.RWMutex
	options        *options
	scheduler      corescheduler.TaskRegistrar
	cleanupRunning atomic.Bool // Guards against concurrent RunCleanup calls.

	schedulerCleanupRegistered atomic.Bool // Marks if RunCleanup is managed by scheduler.
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

	// In-place compaction: two-pointer technique avoids allocating a new slice.
	n := 0
	for _, reqTime := range b.requests {
		if reqTime.After(cutoff) {
			b.requests[n] = reqTime
			n++
		}
	}
	b.requests = b.requests[:n]
	validRequests := n

	resetTime := uint64(0)
	if len(b.requests) > 0 {
		resetTime = uint64(b.requests[0].Add(period).Unix()) // #nosec G115 -- Unix timestamps are positive
	}

	// Check if rate limit is exceeded
	if int64(validRequests) >= limit {
		// #nosec G115 -- resetTime is Unix seconds, fits int64
		return &storages.LimitInfo{Remaining: 0, Reset: int64(resetTime)}, storages.ErrLimitExceeded
	}

	// Add the bucket if it doesn't exist yet
	if !exists {
		p.buckets[key] = b
	}

	// Record the new request
	b.requests = append(b.requests, now)

	// Calculate remaining after adding this request
	info := &storages.LimitInfo{
		Remaining: max(limit-int64(len(b.requests)), 0),
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
	}

	return nil
}

// Close releases any resources held by the provider.
// No-op since there are no background goroutines to stop.
func (p *Provider) Close() error {
	return nil
}

var _ storages.Storage = (*Provider)(nil)
