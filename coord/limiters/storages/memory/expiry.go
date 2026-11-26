// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"container/heap"
	"math"
	"time"
)

// expiryHeap indexes each bucket once, ordered by when its last request expires.
type expiryHeap []*bucket

func (h expiryHeap) Len() int           { return len(h) }
func (h expiryHeap) Less(i, j int) bool { return h[i].expiresAt.Before(h[j].expiresAt) }
func (h expiryHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].expiryIndex = i
	h[j].expiryIndex = j
}

func (h *expiryHeap) Push(value any) {
	b, _ := value.(*bucket) //nolint:errcheck // only buckets are inserted
	b.expiryIndex = len(*h)
	*h = append(*h, b)
}

func (h *expiryHeap) Pop() any {
	last := len(*h) - 1
	b := (*h)[last]
	b.expiryIndex = -1
	(*h)[last] = nil
	*h = (*h)[:last]
	return b
}

// refreshExpiry reorders b after its window changes, under the shard lock.
func (p *Provider) refreshExpiry(sh *shard, b *bucket) {
	expiry := b.expiry()
	switch {
	case b.expiryIndex < 0:
		b.expiresAt = expiry
		heap.Push(&sh.expiries, b)
	case expiry.Equal(b.expiresAt):
		return
	default:
		b.expiresAt = expiry
		heap.Fix(&sh.expiries, b.expiryIndex)
	}
	p.lowerExpiry(expiry)
}

// reclaimLocked frees up to needed empty windows and refreshes the global
// expiry bound. The caller holds all shard locks. Each shard contributes at
// most one inspected head in addition to the buckets actually removed.
func (p *Provider) reclaimLocked(now time.Time, needed int) int {
	next := int64(math.MaxInt64)
	freed := 0
	for i := range p.shards {
		sh := &p.shards[i]
		for len(sh.expiries) > 0 {
			b := sh.expiries[0]
			if p.visitReclaimBucket != nil {
				p.visitReclaimBucket()
			}
			if needed == 0 || b.expiresAt.After(now) {
				next = min(next, p.offset(b.expiresAt))
				break
			}
			key, _ := b.lruElem.Value.(string) //nolint:errcheck // lru holds only string keys
			p.removeLocked(sh, key)
			needed--
			freed++
		}
	}
	p.nextExpiry.Store(next)
	return freed
}
