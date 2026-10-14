// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package probfilterit_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/probfilter"
	"github.com/altessa-s/go-atlas/data/probfilter/factory"

	probfilterconfig "github.com/altessa-s/go-atlas/config/probfilter"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
)

// TestFactory_BuildsRedisFilters builds a Bloom and a Cuckoo filter on Redis
// through the factory, as an application wires them from configuration. The
// build checks the server's eviction policy for real, and the Bloom filter is
// rebuilt from its loader inside Build.
func TestFactory_BuildsRedisFilters(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	ctx := t.Context()

	policy, err := fx.admin.ConfigGet(ctx, "maxmemory-policy").Result()
	require.NoError(t, err)
	if strings.HasPrefix(policy["maxmemory-policy"], "allkeys-") {
		t.Skipf("the server's maxmemory-policy %q may evict filter keys; the factory refuses it", policy["maxmemory-policy"])
	}

	defaults := probfilterconfig.NewDefaults()
	onRedis := &probfilterconfig.Storage{
		Type:  probfilterconfig.StorageTypeRedis,
		Redis: &storageconfig.RedisConfig{KeysPrefix: fx.prefix},
	}
	noCron, onStart := "", true

	snapshot := values("seed", 200)
	built, err := factory.NewFilter(filterName, &probfilterconfig.Filter{
		Type: probfilterconfig.TypeBloom,
		Bloom: &probfilterconfig.BloomConfig{
			Storage:        onRedis,
			ExpectedItems:  1000,
			RebuildCron:    &noCron,
			RebuildOnStart: &onStart,
		},
	}, &defaults).
		UseRedisClient(redisClient(t)).
		UseDataLoader(openLoader(snapshot, true)).
		Build()
	require.NoError(t, err)
	bf, ok := built.(probfilter.RebuildableFilter)
	require.True(t, ok)
	t.Cleanup(func() { closeIfCloser(built) })

	// The filter Build rebuilt is the one every other process opens.
	peer := fx.bloom(t)
	requirePresent(t, peer, snapshot...)
	require.False(t, bf.LastRebuild().IsZero())
	committed, err := peer.RebuildCommitted(ctx)
	require.NoError(t, err)
	require.True(t, committed)

	// A Cuckoo filter under its own name, in the same namespace.
	built, err = factory.NewFilter("sessions", &probfilterconfig.Filter{
		Type:   probfilterconfig.TypeCuckoo,
		Cuckoo: &probfilterconfig.CuckooConfig{Storage: onRedis, Capacity: 1000},
	}, &defaults).
		UseRedisClient(redisClient(t)).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { closeIfCloser(built) })
	cf, ok := built.(probfilter.DeletableFilter)
	require.True(t, ok)

	require.NoError(t, cf.Add(ctx, "session:1"))
	requirePresent(t, cf, "session:1")
	deleted, err := cf.Delete(ctx, "session:1")
	require.NoError(t, err)
	require.True(t, deleted)
	requireAbsent(t, cf, "session:1")
	require.NotEmpty(t, fx.keys(t, fx.prefix+"sessions"), "the Cuckoo filter must live under the configured prefix")
}

// closeIfCloser closes a filter built by the factory.
func closeIfCloser(f probfilter.Filter) {
	if c, ok := f.(interface {
		Close(ctx context.Context) error
	}); ok {
		_ = c.Close(context.Background())
	}
}
