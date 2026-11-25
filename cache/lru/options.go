// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lru

//go:generate go tool optgen generate --type=options

type options struct {
	// WithShardCount requests a positive power-of-two shard count. Other values
	// select the CPU-based default. NewShardedCache may halve the requested or
	// default count repeatedly to give each shard at least 64 slots, down to
	// one shard if needed. See [NewShardedCache] for the eviction guarantee.
	shardCount int
}
