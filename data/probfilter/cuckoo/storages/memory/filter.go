// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"hash/maphash"
	"math/bits"
	"math/rand/v2"
)

const (
	// bucketSize is the number of fingerprint slots per bucket.
	bucketSize = 4
	// maxKicks bounds the relocation walk of one insert.
	maxKicks = 500
	// fingerprintShift selects the top byte of the 64-bit hash as fingerprint.
	fingerprintShift = 56
	// fingerprintValues is the number of non-zero fingerprint values.
	fingerprintValues = 255
	// altIndexMix spreads a fingerprint over the bucket index range for the
	// partial-key alternate index (a 32-bit golden-ratio multiplier).
	altIndexMix = 0x9e3779b1
)

// fingerprint is an 8-bit item fingerprint; 0 marks an empty slot.
type fingerprint uint8

type bucket [bucketSize]fingerprint

// kick records one relocation step so a failed insert can be undone.
type kick struct {
	bucket uint64
	slot   int
	prev   fingerprint
}

// cuckooFilter is a partial-key cuckoo filter with 4-slot buckets and 8-bit
// fingerprints. Unlike the seiflotfy/cuckoofilter library it replaces, a failed
// insert undoes its relocation walk, so it never evicts an existing member:
// every added (and not deleted) value keeps answering true from lookup.
// It is not safe for concurrent use; [Storage] serializes access.
type cuckooFilter struct {
	buckets []bucket
	mask    uint64
	count   uint
	seed    maphash.Seed
	kicks   []kick // reused relocation log
}

// newCuckooFilter allocates a filter with at least capacity slots: the bucket
// count is the next power of two of capacity/bucketSize, minimum one.
func newCuckooFilter(capacity uint) *cuckooFilter {
	n := uint64(capacity+bucketSize-1) / bucketSize
	if n < 1 {
		n = 1
	}
	n = 1 << bits.Len64(n-1)
	return &cuckooFilter{
		buckets: make([]bucket, n),
		mask:    n - 1,
		seed:    maphash.MakeSeed(),
	}
}

// slots returns the total number of fingerprint slots.
func (f *cuckooFilter) slots() uint {
	return uint(len(f.buckets)) * bucketSize
}

// locate returns the fingerprint and both candidate buckets of value.
func (f *cuckooFilter) locate(value string) (fingerprint, uint64, uint64) {
	h := maphash.String(f.seed, value)
	fp := fingerprint(h>>fingerprintShift)%fingerprintValues + 1 // never 0, which marks an empty slot
	i1 := h & f.mask
	return fp, i1, f.alt(i1, fp)
}

// alt returns the other candidate bucket of fp stored in bucket i. It is an
// involution: alt(alt(i, fp), fp) == i, so an entry can be relocated without
// knowing the original value.
func (f *cuckooFilter) alt(i uint64, fp fingerprint) uint64 {
	return (i ^ uint64(uint32(fp)*altIndexMix)) & f.mask
}

// lookup reports whether value might be in the filter.
func (f *cuckooFilter) lookup(value string) bool {
	fp, i1, i2 := f.locate(value)
	return f.buckets[i1].has(fp) || f.buckets[i2].has(fp)
}

// insert adds value. It returns false — leaving the filter exactly as it
// was — when no slot can be freed within maxKicks relocations.
func (f *cuckooFilter) insert(value string) bool {
	fp, i1, i2 := f.locate(value)
	if f.buckets[i1].put(fp) || f.buckets[i2].put(fp) {
		f.count++
		return true
	}

	f.kicks = f.kicks[:0]
	i := i1
	if rand.N(2) == 1 { //nolint:mnd // pick one of the two candidate buckets
		i = i2
	}
	for range maxKicks {
		slot := rand.IntN(bucketSize)
		victim := f.buckets[i][slot]
		f.kicks = append(f.kicks, kick{bucket: i, slot: slot, prev: victim})
		f.buckets[i][slot] = fp

		fp = victim
		i = f.alt(i, fp)
		if f.buckets[i].put(fp) {
			f.count++
			return true
		}
	}

	// Undo the walk in reverse: every displaced fingerprint returns to its
	// slot and the new fingerprint, never committed, disappears.
	for j := len(f.kicks) - 1; j >= 0; j-- {
		k := f.kicks[j]
		f.buckets[k.bucket][k.slot] = k.prev
	}
	return false
}

// remove deletes one copy of value's fingerprint and reports whether one was
// found. Removing a value that was never added can remove the fingerprint of
// a colliding member.
func (f *cuckooFilter) remove(value string) bool {
	fp, i1, i2 := f.locate(value)
	if f.buckets[i1].clear(fp) || f.buckets[i2].clear(fp) {
		f.count--
		return true
	}
	return false
}

func (b *bucket) has(fp fingerprint) bool {
	for _, v := range b {
		if v == fp {
			return true
		}
	}
	return false
}

func (b *bucket) put(fp fingerprint) bool {
	for i, v := range b {
		if v == 0 {
			b[i] = fp
			return true
		}
	}
	return false
}

func (b *bucket) clear(fp fingerprint) bool {
	for i, v := range b {
		if v == fp {
			b[i] = 0
			return true
		}
	}
	return false
}
