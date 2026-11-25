// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisfilter

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/cache/probfilter"
	"github.com/altessa-s/go-atlas/internal/redis/redisutils"
)

// CheckEvictionPolicy checks every primary for an eviction policy that can
// discard a shared filter. Only an explicit CONFIG permission denial on a
// verified set of primaries returns false without an error.
func CheckEvictionPolicy(ctx context.Context, client redis.UniversalClient) (bool, error) {
	switch c := client.(type) {
	case *redis.Client:
		return standalonePolicy(ctx, c)
	case *redis.ClusterClient:
		if c == nil {
			return false, probfilter.ErrUnverifiableTopology
		}
		return clusterPolicy(ctx, c)
	default:
		return false, fmt.Errorf("%w: unsupported client %T", probfilter.ErrUnverifiableTopology, client)
	}
}

func standalonePolicy(ctx context.Context, c *redis.Client) (bool, error) {
	if err := redisutils.VerifyStandalonePrimary(ctx, c); err != nil {
		return false, fmt.Errorf("%w: %w", probfilter.ErrUnverifiableTopology, err)
	}
	return checkPolicy(ctx, c)
}

// CheckSentinelEvictionPolicy checks a client explicitly identified by its
// owner as the result of NewFailoverClusterClient.
func CheckSentinelEvictionPolicy(ctx context.Context, c *redis.ClusterClient) (bool, error) {
	if c == nil || c.Options().ClusterSlots == nil {
		return false, probfilter.ErrUnverifiableTopology
	}
	// The failover constructor's slot callback queries Sentinel for the current
	// primary. Compare that independent answer with the actual routing client.
	slots, err := c.Options().ClusterSlots(ctx)
	if err != nil {
		return false, fmt.Errorf("%w: Sentinel discovery: %w", probfilter.ErrUnverifiableTopology, err)
	}
	if len(slots) != 1 || slots[0].Start != 0 || slots[0].End != 16383 || len(slots[0].Nodes) == 0 {
		return false, probfilter.ErrUnverifiableTopology
	}
	primary, err := c.MasterForKey(ctx, "")
	if err != nil {
		return false, fmt.Errorf("%w: Sentinel primary: %w", probfilter.ErrUnverifiableTopology, err)
	}
	if primary.NodeAddress() != slots[0].Nodes[0].Addr {
		return false, fmt.Errorf("%w: Sentinel primary differs from routing state", probfilter.ErrUnverifiableTopology)
	}
	return standalonePolicy(ctx, primary)
}

func clusterPolicy(ctx context.Context, c *redis.ClusterClient) (bool, error) {
	shards, err := c.ClusterShards(ctx).Result()
	if err != nil {
		return false, fmt.Errorf("%w: CLUSTER SHARDS: %w", probfilter.ErrUnverifiableTopology, err)
	}
	expected, err := primaryIDs(shards)
	if err != nil {
		return false, err
	}
	// ForEachMaster can fall back to cached routing data. Use it only to
	// obtain driver-owned clients and match their identities to fresh SHARDS.
	var mu sync.Mutex
	var primaries []*redis.Client
	if err := c.ForEachMaster(ctx, func(_ context.Context, node *redis.Client) error {
		mu.Lock()
		defer mu.Unlock()
		primaries = append(primaries, node)
		return nil
	}); err != nil {
		return false, fmt.Errorf("%w: enumerate driver primaries: %w", probfilter.ErrUnverifiableTopology, err)
	}
	if len(primaries) != len(expected) {
		return false, probfilter.ErrUnverifiableTopology
	}
	for _, node := range primaries {
		id, err := node.ClusterMyID(ctx).Result()
		if err != nil {
			return false, fmt.Errorf("%w: CLUSTER MYID: %w", probfilter.ErrUnverifiableTopology, err)
		}
		if !expected[id] {
			return false, fmt.Errorf("%w: primary identity differs from CLUSTER SHARDS", probfilter.ErrUnverifiableTopology)
		}
		delete(expected, id)
	}
	verified := true
	var failures []error
	for _, node := range primaries {
		ok, err := checkPolicy(ctx, node)
		verified = verified && ok
		if err != nil {
			failures = append(failures, err)
		}
	}
	policyErr := errors.Join(failures...)
	if errors.Is(policyErr, probfilter.ErrUnsafeEvictionPolicy) {
		return true, policyErr
	}
	return verified, policyErr
}

func primaryIDs(shards []redis.ClusterShard) (map[string]bool, error) {
	expected := make(map[string]bool, len(shards))
	for _, shard := range shards {
		var id string
		for _, node := range shard.Nodes {
			if node.Role == "master" {
				if node.ID == "" || node.Health != "online" || id != "" {
					return nil, probfilter.ErrUnverifiableTopology
				}
				id = node.ID
			}
		}
		if id == "" || expected[id] {
			return nil, probfilter.ErrUnverifiableTopology
		}
		expected[id] = true
	}
	if len(expected) == 0 {
		return nil, probfilter.ErrUnverifiableTopology
	}
	return expected, nil
}

func checkPolicy(ctx context.Context, c *redis.Client) (bool, error) {
	values, err := c.ConfigGet(ctx, "maxmemory-policy").Result()
	if redis.HasErrorPrefix(err, "NOPERM ") {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("CONFIG GET maxmemory-policy: %w", err)
	}
	policy := values["maxmemory-policy"]
	if policy == "" {
		return false, errors.New("CONFIG GET missing maxmemory-policy")
	}
	if strings.HasPrefix(policy, "allkeys-") {
		return true, fmt.Errorf("%w on %s", probfilter.ErrUnsafeEvictionPolicy, c.Options().Addr)
	}
	return true, nil
}
