// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"iter"
	"slices"
	"sync"
	"testing"

	"github.com/alicebob/miniredis/v2/server"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/cache/probfilter"
	"github.com/altessa-s/go-atlas/factory/cache/probfilter/cuckoo/storages/redis"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/redistest"

	cuckooredis "github.com/altessa-s/go-atlas/cache/probfilter/cuckoo/storages/redis"
	probfilterconfig "github.com/altessa-s/go-atlas/config/probfilter"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	probfilterfactory "github.com/altessa-s/go-atlas/factory/cache/probfilter"
)

func redisCuckooCfg() *probfilterconfig.Filter {
	return &probfilterconfig.Filter{
		Type: probfilterconfig.TypeCuckoo,
		Cuckoo: &probfilterconfig.CuckooConfig{
			Storage:            &probfilterconfig.Storage{Type: probfilterconfig.StorageTypeRedis},
			Capacity:           100,
			CapacityMultiplier: new(2.5),
		},
	}
}

func defaults() *probfilterconfig.Defaults {
	d := probfilterconfig.NewDefaults()
	return &d
}

func TestBuilder_ExpansionAndPrefix(t *testing.T) {
	t.Parallel()
	client, mr := redistest.RedisClient(t)
	redistest.ServeTopology(mr, "master", "0")

	var mu sync.Mutex
	var reserves [][]string
	require.NoError(t, mr.Server().Register("CF.RESERVE", func(c *server.Peer, _ string, args []string) {
		mu.Lock()
		defer mu.Unlock()
		reserves = append(reserves, slices.Clone(args))
		c.WriteOK()
	}))
	require.NoError(t, mr.Server().Register("CF.INSERT", func(c *server.Peer, _ string, _ []string) { c.WriteInt(1) }))

	cfg := redisCuckooCfg()
	cfg.Cuckoo.Storage.Redis = &storageconfig.RedisConfig{KeysPrefix: "svc:"}
	f, err := probfilterfactory.NewFilter("sessions", cfg, defaults()).
		SkipEvictionPolicyCheck(true).
		UseCuckooStorage(probfilterconfig.StorageTypeRedis, factory.Builder(client)).
		Build(t.Context())
	require.NoError(t, err)

	// The staging reserve carries the configured expansion. miniredis cannot
	// materialize the module value, so the final RENAME fails; only the
	// reserve arguments matter here.
	loader := probfilter.NewDataLoader(func() iter.Seq[string] { return slices.Values([]string{"a"}) })
	_ = f.(probfilter.RebuildableFilter).Rebuild(t.Context(), loader)

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, reserves)
	require.Contains(t, reserves[0][0], "{svc:sessions}", "the key carries the configured prefix and the filter name")
	require.Equal(t, []string{"100", "EXPANSION", "3"}, reserves[0][1:], "ceil(2.5) = 3")
}

func TestBuilder_ClientRequired(t *testing.T) {
	t.Parallel()

	_, err := factory.Builder(nil)(t.Context(), probfilterfactory.CuckooStorageSpec{})
	require.ErrorIs(t, err, factory.ErrClientRequired)

	client, _ := redistest.RedisClient(t)
	storage, err := factory.Builder(client)(t.Context(), probfilterfactory.CuckooStorageSpec{
		Name: "sessions", SkipEvictionPolicyCheck: true,
		Capacity: 100,
	})
	require.NoError(t, err)
	require.IsType(t, &cuckooredis.Storage{}, storage)
}

func TestBuilder_UnsafeEvictionPolicyFailsBuild(t *testing.T) {
	t.Parallel()
	client, mr := redistest.RedisClient(t)
	redistest.ServeTopology(mr, "master", "0")
	require.NoError(t, mr.Server().Register("CONFIG", func(c *server.Peer, _ string, _ []string) {
		c.WriteMapLen(1)
		c.WriteBulk("maxmemory-policy")
		c.WriteBulk("allkeys-lru")
	}))

	_, err := probfilterfactory.NewFilter("sessions", redisCuckooCfg(), defaults()).
		UseCuckooStorage(probfilterconfig.StorageTypeRedis, factory.Builder(client)).
		Build(t.Context())
	require.ErrorIs(t, err, probfilter.ErrUnsafeEvictionPolicy)

	_, err = probfilterfactory.NewFilter("sessions", redisCuckooCfg(), defaults()).
		UseCuckooStorage(probfilterconfig.StorageTypeRedis, factory.Builder(client)).
		SkipEvictionPolicyCheck(true).
		Build(t.Context())
	require.NoError(t, err)
}
