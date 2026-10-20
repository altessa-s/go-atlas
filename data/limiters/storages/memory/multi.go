// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"context"
	"slices"
	"time"

	"github.com/altessa-s/go-atlas/core/runtime/panics"
	"github.com/altessa-s/go-atlas/data/limiters/storages"
)

// smallCall is the number of limits a multi-key call handles in stack
// buffers.
const smallCall = 8

// AllowAll admits one request against every limit atomically; see
// [storages.MultiStorage.AllowAll]. It locks the shards of the keys in index
// order, checks every window, and takes a slot on each only when all admit.
//
// New keys need a bucket each. When the free slots fall short the call locks
// every shard and reclaims expired windows; if that is not enough,
// [EvictionReject] fails the call with a [*storages.KeyError] for the first
// new key left without a bucket that wraps [ErrCapacityExceeded], and
// [EvictionLRU] evicts least recently used buckets other than the call's
// own. A call with more limits than [WithMaxBuckets] cannot be tracked at
// once and fails the same way. The failure names the first new key left
// without room. A refusal or a capacity failure takes no slot and inserts no
// bucket. New keys reserve their slots in one step, so two calls racing for
// the last slots never both fail when either fits.
func (p *Provider) AllowAll(ctx context.Context, limits []storages.Limit) ([]storages.LimitInfo, error) {
	panics.MustNonNil(ctx, "context must not be nil")
	if err := storages.ValidateLimits(limits); err != nil {
		return nil, err
	}
	var idxBuf, orderBuf [smallCall]int
	var bucketBuf [smallCall]*bucket
	idx, order, buckets := idxBuf[:0], orderBuf[:0], bucketBuf[:0]
	if len(limits) > smallCall {
		idx = make([]int, 0, len(limits))
		order = make([]int, 0, len(limits))
		buckets = make([]*bucket, 0, len(limits))
	}
	for _, l := range limits {
		idx = append(idx, p.shardIndex(l.Key))
		buckets = append(buckets, nil)
	}
	order = append(order, idx...)
	slices.Sort(order)
	order = slices.Compact(order)

	all := len(order) == len(p.shards)
	for _, i := range order {
		p.shards[i].mu.Lock()
	}
	infos, retry, err := p.allowAllLocked(limits, idx, buckets, all)
	for _, i := range order {
		p.shards[i].mu.Unlock()
	}
	if !retry {
		return infos, err
	}

	p.lockAll()
	defer p.unlockAll()
	infos, _, err = p.allowAllLocked(limits, idx, buckets, true)
	return infos, err
}

// allowAllLocked runs [Provider.AllowAll] with the shards in idx locked, or
// every shard when all is set. Without every shard it cannot reclaim or
// evict, so when new keys find too few free slots it reserves nothing and
// reports retry, unless [EvictionReject] already knows the answer. buckets
// is scratch space of len(limits).
func (p *Provider) allowAllLocked(limits []storages.Limit, idx []int, buckets []*bucket, all bool) (
	infos []storages.LimitInfo, retry bool, err error,
) {
	now := time.Now()
	missing := p.resolveLocked(limits, idx, buckets)
	if all && missing > p.freeSlots() && p.purgeDue(now) {
		p.purgeLocked(now)
		missing = p.resolveLocked(limits, idx, buckets)
	}

	// Check every window before taking anything. A new key has an empty
	// window, which admits any positive limit.
	infos = make([]storages.LimitInfo, len(limits))
	refused := -1
	for i, b := range buckets {
		if b == nil {
			infos[i] = storages.LimitInfo{Remaining: limits[i].Limit}
			continue
		}
		info, ok := b.apply(now, limits[i].Limit, limits[i].Period, false)
		infos[i] = info
		if limits[i].Period != b.window {
			// A shorter period than the window was recorded with can empty
			// it; keep it reclaimable even when this call fails below.
			p.lowerExpiry(b.expiry())
		}
		if !ok && refused < 0 {
			refused = i
		}
	}
	if refused >= 0 {
		// A refused request still counts as use, as in Allow.
		for i, b := range buckets {
			if b != nil {
				infos[i], _ = p.touchAtLocked(&p.shards[idx[i]], b, now, limits[i].Limit, limits[i].Period, false)
			}
		}
		return infos, false, &storages.KeyError{Index: refused, Key: limits[refused].Key, Err: storages.ErrLimitExceeded}
	}

	// Without every shard lock the reservation is all or nothing; with them
	// nobody else reserves, so a partial one is never seen.
	reserved := p.reserveSlots(missing, all)
	if reserved < missing {
		reject := p.options.eviction == EvictionReject
		switch {
		case !all && (!reject || p.purgeDue(now)):
			return nil, true, nil
		case reject:
			room := reserved
			if !all {
				// Slots freed since the failed reservation do not make
				// the call fit retroactively; it still names a key.
				room = min(p.freeSlots(), missing-1)
			}
			p.releaseSlots(reserved)
			return nil, false, capacityError(limits, buckets, room)
		}
		// LRU may evict every bucket outside the call, but not the call's
		// own: new keys fit only into the slots the existing ones leave.
		if room := p.options.maxBuckets - (len(limits) - missing); missing > room {
			p.releaseSlots(reserved)
			return nil, false, capacityError(limits, buckets, room)
		}
	}

	// Existing keys first: touching them makes them the most recently used,
	// so the evictions below never pick a bucket of this call.
	for i, b := range buckets {
		if b != nil {
			infos[i], _ = p.touchAtLocked(&p.shards[idx[i]], b, now, limits[i].Limit, limits[i].Period, true)
		}
	}
	for i, b := range buckets {
		if b != nil {
			continue
		}
		if reserved > 0 {
			reserved--
		} else {
			p.evictLRULocked()
		}
		sh := &p.shards[idx[i]]
		infos[i], _ = p.touchAtLocked(sh, p.insertLocked(sh, limits[i].Key), now, limits[i].Limit, limits[i].Period, true)
	}
	return infos, false, nil
}

// resolveLocked looks up the bucket of every limit into buckets and returns
// how many keys have none. The caller MUST hold the shards in idx.
func (p *Provider) resolveLocked(limits []storages.Limit, idx []int, buckets []*bucket) int {
	missing := 0
	for i, l := range limits {
		buckets[i] = p.shards[idx[i]].buckets[l.Key]
		if buckets[i] == nil {
			missing++
		}
	}
	return missing
}

// capacityError reports the first new key that finds no room once room
// new keys before it have one.
func capacityError(limits []storages.Limit, buckets []*bucket, room int) error {
	for i, b := range buckets {
		if b != nil {
			continue
		}
		if room <= 0 {
			return &storages.KeyError{Index: i, Key: limits[i].Key, Err: ErrCapacityExceeded}
		}
		room--
	}
	return ErrCapacityExceeded
}
