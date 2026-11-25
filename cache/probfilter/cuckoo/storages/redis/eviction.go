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
// Cuckoo filter depends on: the filter, its rebuild metadata and delete
// records carry no TTL, so an allkeys-* maxmemory-policy may drop them and
// silently break the rebuild and delete guarantees. Every master of a cluster
// is checked.
//
// It returns an error wrapping probfilter.ErrUnsafeEvictionPolicy for an
// allkeys-* policy and probfilter.ErrUnverifiableTopology for incomplete
// topology. Only CONFIG permission denial returns false without an error.
func CheckEvictionPolicy(ctx context.Context, client redis.UniversalClient) (bool, error) {
	return redisfilter.CheckEvictionPolicy(ctx, client)
}

// CheckSentinelEvictionPolicy checks the current primary of a client built
// with redis.NewFailoverClusterClient, using synchronous Sentinel discovery.
func CheckSentinelEvictionPolicy(ctx context.Context, client *redis.ClusterClient) (bool, error) {
	return redisfilter.CheckSentinelEvictionPolicy(ctx, client)
}
