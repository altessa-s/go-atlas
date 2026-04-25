// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

// DefaultKeyPrefix is the default prefix for Redis keys.
const DefaultKeyPrefix = "cuckoo:"

// DefaultCapacity is the default capacity of the filter.
const DefaultCapacity int64 = 100000

// options contains Redis Cuckoo filter storage configuration.
type options struct {
	keyPrefix string `optgen:"default=DefaultKeyPrefix"`
	capacity  int64  `optgen:"default=DefaultCapacity" optval:"positive"`
}
