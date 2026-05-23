// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

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
}
