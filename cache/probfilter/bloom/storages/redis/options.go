// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

//go:generate go tool optgen generate

// DefaultKeyPrefix is the default prefix for Redis keys.
const DefaultKeyPrefix = "bloom:"

// DefaultExpectedItems is the default expected number of items.
const DefaultExpectedItems int64 = 100000

// DefaultFalsePositiveRate is the default target false positive rate.
const DefaultFalsePositiveRate = 0.01

// options contains Redis Bloom filter storage configuration.
type options struct {
	keyPrefix         string  `optgen:"default=DefaultKeyPrefix"`
	expectedItems     int64   `optgen:"default=DefaultExpectedItems" optval:"positive"`
	falsePositiveRate float64 `optgen:"default=DefaultFalsePositiveRate" optval:"positive=allow_zero"`
	// WithInsecureTreatMissingAsEmpty makes a lookup of a filter that vanished
	// from Redis after this process saw it (a flush, an eviction or a manual
	// delete) answer false instead of failing with
	// [probfilter.ErrFilterLost]. Without it such a lookup fails closed, so the
	// caller reads the authoritative store until a rebuild publishes the filter
	// again. With it every lookup of a value the filter held answers "absent"
	// until then, which is a false negative. Use it only when losing the
	// filter must not add load to the authoritative store and the caller
	// tolerates false negatives.
	insecureTreatMissingAsEmpty bool
}
