// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"container/list"
	"context"
	"hash/maphash"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/altessa-s/go-atlas/core/runtime/panics"
	"github.com/altessa-s/go-atlas/data/limiters/storages"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// bucket represents a sliding window bucket for rate limiting.
type bucket struct {
	// requests holds request timestamps in non-decreasing order (time.Now is
	// monotonic and appends happen under the shard lock). Live entries are
	// requests[head:]; the prefix before head is expired and reclaimed by
	// amortized compaction in touchLocked.
	requests []time.Time
	head     int
	limit    int64
	window   time.Duration
	lastUsed time.Time
	// recency is the provider-wide touch sequence; the smallest value across
	// the shard tails is the global least-recently-used bucket.
	recency uint64
	// lruElem is the bucket's node in its shard's lru; its Value is the key.
	lruElem *list.Element
}

// shard owns a slice of the key space with an exact local LRU, most recent
// at the front. Shards have no quota of their own: the bucket cap and LRU
// eviction stay global.
type shard struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	lru     *list.List
	_       [64]byte // keep neighboring shard locks off one cache line
}

// Provider implements an in-memory rate limiting provider using sliding window algorithm.
// Use RunCleanup() to remove expired buckets, either manually or via scheduler.
//
// Keys are spread over shards so concurrent Allow calls on existing keys do
// not share a lock. Below the bucket cap a new key is inserted under its own
// shard lock; at the cap the insert locks every shard so the evicted bucket
// is exactly the least recently used one across the whole provider.
type Provider struct {
	shards []shard // see WithShards
	seed   maphash.Seed
	// sequence orders touches across shards; it is advanced under the
	// touched bucket's shard lock, so each shard's LRU follows it.
	sequence atomic.Uint64
	// count is the number of tracked buckets plus in-flight reservations, so
	// buckets <= count <= maxBuckets at all times. Inserts below the cap
	// reserve a slot with CAS under one shard lock; eviction hands its slot to
	// the new bucket without changing count.
	count       atomic.Int64
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
		shards:    make([]shard, max(opts.shards, 1)),
		options:   opts,
		seed:      maphash.MakeSeed(),
		scheduler: opts.scheduler,
	}
	for i := range p.shards {
		p.shards[i].buckets = make(map[string]*bucket)
		p.shards[i].lru = list.New()
	}

	// Register cleanup task with scheduler if provided
	_ = p.registerCleanupTask(opts.cleanupSchedule) //nolint:errcheck // task registration is optional

	return p
}

func (p *Provider) shardFor(key string) *shard {
	if len(p.shards) == 1 {
		return &p.shards[0]
	}
	return &p.shards[maphash.String(p.seed, key)%uint64(len(p.shards))]
}

// Allow checks if a request should be allowed based on the rate limit using sliding window algorithm.
func (p *Provider) Allow(ctx context.Context, key string, limit int64, period time.Duration) (*storages.LimitInfo, error) {
	panics.MustNonNil(ctx, "context must not be nil")
	panics.Must(key != "", "key must not be empty")
	panics.Must(limit > 0, "limit must be greater than 0")
	panics.Must(period > 0, "period must be greater than 0")

	sh := p.shardFor(key)
	sh.mu.Lock()
	if b, ok := sh.buckets[key]; ok {
		info, err := p.touchLocked(sh, b, limit, period)
		sh.mu.Unlock()
		return info, err
	}
	if p.reserveSlot() {
		// Below the cap: insert under this shard's lock alone.
		info, err := p.touchLocked(sh, p.insertLocked(sh, key), limit, period)
		sh.mu.Unlock()
		return info, err
	}
	if len(p.shards) == 1 {
		// The only shard lock is already every shard lock.
		p.evictLRULocked()
		info, err := p.touchLocked(sh, p.insertLocked(sh, key), limit, period)
		sh.mu.Unlock()
		return info, err
	}
	sh.mu.Unlock()

	// At the cap a new key evicts the global least-recently-used bucket,
	// which may live in any shard, so this path holds every shard lock.
	p.lockAll()
	defer p.unlockAll()
	if b, ok := sh.buckets[key]; ok {
		return p.touchLocked(sh, b, limit, period)
	}
	if p.reserveSlot() {
		// Cleanup freed a slot meanwhile.
		return p.touchLocked(sh, p.insertLocked(sh, key), limit, period)
	}
	p.evictLRULocked()
	return p.touchLocked(sh, p.insertLocked(sh, key), limit, period)
}

// reserveSlot claims room for one more bucket, failing when the cap is
// reached. An unlimited provider always succeeds.
func (p *Provider) reserveSlot() bool {
	maxBuckets := int64(p.options.maxBuckets)
	for {
		c := p.count.Load()
		if maxBuckets > 0 && c >= maxBuckets {
			return false
		}
		if p.count.CompareAndSwap(c, c+1) {
			return true
		}
	}
}

// insertLocked adds a new bucket for key to sh. Its slot in count is already
// held by the caller. The caller MUST hold sh.mu.
func (p *Provider) insertLocked(sh *shard, key string) *bucket {
	b := &bucket{requests: make([]time.Time, 0, 16)} //nolint:mnd // reasonable initial capacity for sliding window
	b.lruElem = sh.lru.PushFront(key)
	sh.buckets[key] = b
	return b
}

// touchLocked marks b most recently used and applies one request to its
// sliding window. The caller MUST hold sh.mu. The clock is read under the
// lock so lastUsed and request timestamps are non-decreasing in lock order:
// each shard's LRU then equals its lastUsed order, which lets
// runCleanupCycle stop at the first non-idle tail bucket.
func (p *Provider) touchLocked(sh *shard, b *bucket, limit int64, period time.Duration) (*storages.LimitInfo, error) {
	now := time.Now()
	b.recency = p.sequence.Add(1)
	sh.lru.MoveToFront(b.lruElem)
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

	// Record the new request
	b.requests = append(b.requests, now)

	return &storages.LimitInfo{
		Remaining: max(limit-int64(len(b.requests)-b.head), 0),
		Reset:     int64(resetTime), // #nosec G115 -- resetTime is Unix seconds, fits int64
	}, nil
}

// Reset resets the rate limit for a specific key.
func (p *Provider) Reset(ctx context.Context, key string) error {
	panics.MustNonNil(ctx, "context must not be nil")
	panics.Must(key != "", "key must not be empty")

	sh := p.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()

	// Reset clears the window without refreshing recency.
	if b, exists := sh.buckets[key]; exists {
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

// lockAll takes every shard lock in index order; unlockAll releases them.
func (p *Provider) lockAll() {
	for i := range p.shards {
		p.shards[i].mu.Lock()
	}
}

func (p *Provider) unlockAll() {
	for i := range p.shards {
		p.shards[i].mu.Unlock()
	}
}

// evictLRULocked drops the globally least-recently-used bucket: the shard
// tail with the smallest recency. Its slot passes to the caller's new bucket,
// so count is unchanged. The caller MUST hold every shard lock.
func (p *Provider) evictLRULocked() {
	var oldest *shard
	var oldestRecency uint64
	for i := range p.shards {
		sh := &p.shards[i]
		back := sh.lru.Back()
		if back == nil {
			continue
		}
		key, _ := back.Value.(string) //nolint:errcheck // lru holds only string keys
		if r := sh.buckets[key].recency; oldest == nil || r < oldestRecency {
			oldest, oldestRecency = sh, r
		}
	}
	if oldest != nil {
		key, _ := oldest.lru.Back().Value.(string) //nolint:errcheck // lru holds only string keys
		oldest.drop(key)
	}
}

// removeLocked drops key from sh and releases its slot. The caller MUST
// hold sh.mu; count is atomic, so no other shard lock is needed.
func (p *Provider) removeLocked(sh *shard, key string) {
	if sh.drop(key) {
		p.count.Add(-1)
	}
}

// drop removes key from the shard's map and LRU list, reporting whether it
// was present. The caller MUST hold sh.mu.
func (sh *shard) drop(key string) bool {
	b, ok := sh.buckets[key]
	if ok {
		sh.lru.Remove(b.lruElem)
		delete(sh.buckets, key)
	}
	return ok
}

var _ storages.Storage = (*Provider)(nil)
