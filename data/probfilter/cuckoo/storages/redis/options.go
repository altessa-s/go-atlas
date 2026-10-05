// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

// DefaultKeyPrefix is the default prefix for Redis keys.
const DefaultKeyPrefix = "cuckoo:"

// DefaultCapacity is the default capacity of the filter.
const DefaultCapacity int64 = 100000

// DefaultExpansion is the default sub-filter growth factor; 0 leaves the
// RedisBloom default in effect (the EXPANSION argument is not sent).
const DefaultExpansion int64 = 0

// options contains Redis Cuckoo filter storage configuration.
type options struct {
	keyPrefix string `optgen:"default=DefaultKeyPrefix"`
	capacity  int64  `optgen:"default=DefaultCapacity" optval:"positive"`
	// expansion is the RedisBloom EXPANSION factor: when the filter fills
	// up, RedisBloom adds a sub-filter this many times larger (rounded up to
	// a power of two by RedisBloom). 0 keeps the RedisBloom default.
	expansion int64 `optgen:"default=DefaultExpansion" optval:"positive=allow_zero"`
}
