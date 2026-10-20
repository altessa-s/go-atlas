// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"container/list"
	"context"
	"errors"
	"hash/maphash"
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/altessa-s/go-atlas/core/runtime/panics"
	"github.com/altessa-s/go-atlas/data/limiters/storages"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// ErrCapacityExceeded is returned for a new key when the provider holds
// [WithMaxBuckets] keys, no tracked window has expired and the policy is
// [EvictionReject]. It is not [storages.ErrLimitExceeded]: the key itself is
// within its limit, the provider has no room to track it.
var ErrCapacityExceeded = errors.New("rate limit storage capacity exceeded")

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
// shard lock; at the cap the insert locks every shard, reclaims expired
// windows, and then applies [WithEviction]: the evicted bucket is exactly the
// least recently used one across the whole provider.
//
// Provider implements [storages.MultiStorage]: [Provider.AllowAll] locks the
// shards of its keys in index order, so overlapping calls cannot deadlock.
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
	count atomic.Int64
	// epoch anchors nextExpiry on the monotonic clock.
	epoch time.Time
	// nextExpiry is a lower bound, in nanoseconds since epoch, of the time
	// the first tracked window expires; math.MaxInt64 when none can. Until
	// then a full provider has nothing to reclaim, so it skips the sweep.
	// A window's expiry only grows while its period stays the same, so the
	// bound is lowered only for new buckets, period changes and resets.
	nextExpiry  atomic.Int64
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
		epoch:     time.Now(),
	}
	p.nextExpiry.Store(math.MaxInt64)
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

// shardIndex is the index of [Provider.shardFor].
func (p *Provider) shardIndex(key string) int {
	if len(p.shards) == 1 {
		return 0
	}
	return int(maphash.String(p.seed, key) % uint64(len(p.shards))) // #nosec G115 -- below len(p.shards)
}

// Allow checks if a request should be allowed based on the rate limit using sliding window algorithm.
//
// A new key at [WithMaxBuckets] first reclaims expired windows; when none
// has expired, [EvictionReject] fails it with [ErrCapacityExceeded] and a nil
// LimitInfo, and [EvictionLRU] evicts the least recently used bucket.
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
	if p.options.eviction == EvictionReject && !p.purgeDue(time.Now()) {
		// Nothing to reclaim and nothing to evict: refuse without the
		// all-shard lock, so a flood of new keys stays cheap.
		sh.mu.Unlock()
		return nil, ErrCapacityExceeded
	}
	if len(p.shards) == 1 {
		// The only shard lock is already every shard lock.
		info, err := p.admitAtCapLocked(sh, key, limit, period)
		sh.mu.Unlock()
		return info, err
	}
	sh.mu.Unlock()

	// At the cap a new key reclaims expired windows or evicts the global
	// least-recently-used bucket, which may live in any shard, so this path
	// holds every shard lock.
	p.lockAll()
	defer p.unlockAll()
	if b, ok := sh.buckets[key]; ok {
		return p.touchLocked(sh, b, limit, period)
	}
	if p.reserveSlot() {
		// Cleanup freed a slot meanwhile.
		return p.touchLocked(sh, p.insertLocked(sh, key), limit, period)
	}
	return p.admitAtCapLocked(sh, key, limit, period)
}

// admitAtCapLocked inserts key into a full provider: it reclaims expired
// windows when one may have expired, and otherwise refuses the key under
// [EvictionReject] or evicts the least recently used bucket. The caller MUST
// hold every shard lock and have found no free slot.
func (p *Provider) admitAtCapLocked(sh *shard, key string, limit int64, period time.Duration) (*storages.LimitInfo, error) {
	now := time.Now()
	if !p.reclaimLocked(now) {
		if p.options.eviction == EvictionReject {
			return nil, ErrCapacityExceeded
		}
		p.evictLRULocked()
	}
	// A new bucket has an empty window, which admits any positive limit.
	info, _ := p.touchAtLocked(sh, p.insertLocked(sh, key), now, limit, period, true)
	return &info, nil
}

// reclaimLocked sweeps expired windows when one may have expired by now and
// reserves a slot, reporting whether it got one. The caller MUST hold every
// shard lock.
func (p *Provider) reclaimLocked(now time.Time) bool {
	if !p.purgeDue(now) {
		return false
	}
	p.purgeLocked(now)
	return p.reserveSlot()
}

// reserveSlots reserves n slots in one step and returns n, or 0 when fewer
// are free: another call never sees a part of the reservation. With partial
// set it reserves every free slot up to n instead; only a caller holding
// every shard lock may ask for that, since no one else reserves meanwhile.
func (p *Provider) reserveSlots(n int, partial bool) int {
	maxBuckets := int64(p.options.maxBuckets)
	for {
		c := p.count.Load()
		k := int64(n)
		if maxBuckets > 0 {
			k = min(k, maxBuckets-c)
		}
		if k <= 0 || (k < int64(n) && !partial) {
			return 0
		}
		if p.count.CompareAndSwap(c, c+k) {
			return int(k)
		}
	}
}

// freeSlots returns the number of slots no bucket or reservation holds, or
// math.MaxInt for an unlimited provider.
func (p *Provider) freeSlots() int {
	maxBuckets := int64(p.options.maxBuckets)
	if maxBuckets <= 0 {
		return math.MaxInt
	}
	return int(max(maxBuckets-p.count.Load(), 0))
}

// releaseSlots returns n reserved slots.
func (p *Provider) releaseSlots(n int) {
	if n > 0 {
		p.count.Add(-int64(n))
	}
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
	info, ok := p.touchAtLocked(sh, b, time.Now(), limit, period, true)
	if !ok {
		return &info, storages.ErrLimitExceeded
	}
	return &info, nil
}

// touchAtLocked marks b most recently used at now and applies one request
// to its window, taking a slot when take is set; see [bucket.apply]. The
// caller MUST hold sh.mu and have read now under it.
func (p *Provider) touchAtLocked(sh *shard, b *bucket, now time.Time, limit int64, period time.Duration, take bool) (storages.LimitInfo, bool) {
	b.recency = p.sequence.Add(1)
	sh.lru.MoveToFront(b.lruElem)
	b.lastUsed = now
	b.limit = limit
	reshaped := b.window != period
	b.window = period

	info, ok := b.apply(now, limit, period, take)
	if reshaped {
		// A new bucket or a changed period may expire before the bound.
		p.lowerExpiry(b.expiry())
	}
	return info, ok
}

// apply prunes the expired prefix of b's window at now and checks one
// request against limit. It reports false, with Remaining 0, when the
// window is full. Otherwise it takes a slot when take is set and reports
// the state after it; without take it reports the free slots and leaves the
// window as is, Reset 0 for an empty window. The caller MUST hold the lock
// of b's shard.
func (b *bucket) apply(now time.Time, limit int64, period time.Duration, take bool) (storages.LimitInfo, bool) {
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
		return storages.LimitInfo{Remaining: 0, Reset: int64(resetTime)}, false
	}
	if !take {
		// #nosec G115 -- resetTime is Unix seconds, fits int64
		return storages.LimitInfo{Remaining: limit - int64(validRequests), Reset: int64(resetTime)}, true
	}

	// Record the new request. Into an empty window it is the oldest request,
	// so the window frees its first slot when it expires.
	b.requests = append(b.requests, now)
	if validRequests == 0 {
		resetTime = uint64(now.Add(period).Unix()) // #nosec G115 -- Unix timestamps are positive
	}

	return storages.LimitInfo{
		Remaining: max(limit-int64(len(b.requests)-b.head), 0),
		Reset:     int64(resetTime), // #nosec G115 -- resetTime is Unix seconds, fits int64
	}, true
}

// expiry is the time b's last request leaves its window, after which the
// window is empty and dropping b loses nothing; the zero time for a window
// that is already empty. The caller MUST hold the lock of b's shard.
func (b *bucket) expiry() time.Time {
	if len(b.requests) == b.head {
		return time.Time{}
	}
	return b.requests[len(b.requests)-1].Add(b.window)
}

// expiredAt reports whether b's window is empty at now.
func (b *bucket) expiredAt(now time.Time) bool {
	return !b.expiry().After(now)
}

// offset is t in nanoseconds since p.epoch, on the monotonic clock when t
// carries it; the zero time maps to the minimum.
func (p *Provider) offset(t time.Time) int64 {
	if t.IsZero() {
		return math.MinInt64
	}
	return int64(t.Sub(p.epoch))
}

// lowerExpiry lowers nextExpiry to t when t is earlier.
func (p *Provider) lowerExpiry(t time.Time) {
	v := p.offset(t)
	for {
		cur := p.nextExpiry.Load()
		if v >= cur || p.nextExpiry.CompareAndSwap(cur, v) {
			return
		}
	}
}

// purgeDue reports whether a tracked window may have expired by now.
func (p *Provider) purgeDue(now time.Time) bool {
	return p.offset(now) >= p.nextExpiry.Load()
}

// purgeLocked drops every bucket whose window is empty at now and resets
// nextExpiry to the earliest expiry left. It costs O(buckets), but runs only
// for a full provider and at most once per earliest expiry, unless it
// reclaims slots. The caller MUST hold every shard lock.
func (p *Provider) purgeLocked(now time.Time) {
	next := int64(math.MaxInt64)
	for i := range p.shards {
		sh := &p.shards[i]
		for key, b := range sh.buckets {
			if b.expiredAt(now) {
				p.removeLocked(sh, key)
				continue
			}
			next = min(next, p.offset(b.expiry()))
		}
	}
	p.nextExpiry.Store(next)
}

// Reset resets the rate limit for a specific key.
func (p *Provider) Reset(ctx context.Context, key string) error {
	panics.MustNonNil(ctx, "context must not be nil")
	panics.Must(key != "", "key must not be empty")

	sh := p.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()

	// Reset clears the window without refreshing recency; the empty window
	// can be reclaimed at the cap.
	if b, exists := sh.buckets[key]; exists {
		b.requests = b.requests[:0]
		b.head = 0
		p.lowerExpiry(time.Time{})
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

var _ storages.MultiStorage = (*Provider)(nil)
