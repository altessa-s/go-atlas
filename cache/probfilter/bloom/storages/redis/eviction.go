// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

import (
	"context"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/cache/probfilter/internal/redisfilter"
)

// CheckEvictionPolicy verifies that Redis never evicts the keys a shared
// Bloom filter depends on: the filter and its rebuild metadata carry no TTL,
// so an allkeys-* maxmemory-policy may drop them and silently break the
// rebuild guarantees. Every master of a cluster is checked.
//
// It returns an error wrapping probfilter.ErrUnsafeEvictionPolicy for an
// allkeys-* policy. When the policy cannot be read — managed Redis services
// often deny CONFIG — it returns verified false and no error, leaving the
// decision to the caller.
func CheckEvictionPolicy(ctx context.Context, client redis.UniversalClient) (verified bool, err error) {
	return redisfilter.CheckEvictionPolicy(ctx, client)
}
