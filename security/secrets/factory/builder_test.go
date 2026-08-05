// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/security/secrets"
)

func TestCreateCache(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     config.SecretsCache
		wantNil bool
		wantTyp any
	}{
		{
			// MaxSize 0 documents "use default (1000)", which is the Manager's
			// job — a size-0 LRU would be an error, not a default.
			name:    "unset size defers to the Manager",
			cfg:     config.SecretsCache{MaxSize: 0, ShardCount: 0},
			wantNil: true,
		},
		{
			name:    "negative size defers to the Manager",
			cfg:     config.SecretsCache{MaxSize: -1, ShardCount: 8},
			wantNil: true,
		},
		{
			name:    "no shards yields a standard cache",
			cfg:     config.SecretsCache{MaxSize: 100, ShardCount: 0},
			wantTyp: &secrets.StandardCache[string, *secrets.Value[any]]{},
		},
		{
			name:    "shards yield a sharded cache",
			cfg:     config.SecretsCache{MaxSize: 1000, ShardCount: 8},
			wantTyp: &secrets.ShardedCache[string, *secrets.Value[any]]{},
		},
		{
			// A non-power-of-2 count is corrected by the LRU rather than
			// rejected, so the factory must not treat it as a config error.
			name:    "odd shard count still builds",
			cfg:     config.SecretsCache{MaxSize: 1000, ShardCount: 7},
			wantTyp: &secrets.ShardedCache[string, *secrets.Value[any]]{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cache, err := createCache(tc.cfg)
			require.NoError(t, err)

			if tc.wantNil {
				require.Nil(t, cache)
				return
			}

			require.NotNil(t, cache)
			require.IsType(t, tc.wantTyp, cache)
			require.Zero(t, cache.Len())
		})
	}
}

// The cache must be instantiated for Manager[any]: WithCache takes an `any` and
// the Manager silently falls back to its default when the assertion fails, so a
// wrong instantiation would look like a configured cache that is never used.
func TestCreateCache_UsableByManagerOfAny(t *testing.T) {
	t.Parallel()

	cache, err := createCache(config.SecretsCache{MaxSize: 10})
	require.NoError(t, err)
	require.NotNil(t, cache)

	var asAny any = cache
	_, ok := asAny.(secrets.Cache[string, *secrets.Value[any]])
	require.True(t, ok, "cache must satisfy the interface Manager[any] asserts for")

	cache.Put("key", &secrets.Value[any]{})
	require.Equal(t, 1, cache.Len())
}

func TestBuildManagerOptions_IncludesCacheOnlyWhenConfigured(t *testing.T) {
	t.Parallel()

	base := New(&config.Secrets{})
	withoutCache, err := base.buildManagerOptions()
	require.NoError(t, err)

	sized := New(&config.Secrets{Cache: config.SecretsCache{MaxSize: 100}})
	withCache, err := sized.buildManagerOptions()
	require.NoError(t, err)

	require.Len(t, withCache, len(withoutCache)+1,
		"a configured cache must add exactly one option")
}

func TestBuild_RequiresConfig(t *testing.T) {
	t.Parallel()

	_, err := New(nil).Build(t.Context())
	require.Error(t, err)
}

func TestBuild_RejectsUnsupportedProvider(t *testing.T) {
	t.Parallel()

	_, err := New(&config.Secrets{Provider: "nope"}).Build(t.Context())
	require.Error(t, err)
}

// The memory provider needs no external dependency, so it is the one path that
// exercises config -> provider -> manager end to end.
func TestBuild_MemoryProvider(t *testing.T) {
	t.Parallel()

	manager, err := New(&config.Secrets{
		Provider: config.SecretsProviderMemory,
		Cache:    config.SecretsCache{MaxSize: 64, ShardCount: 4},
	}).Build(t.Context())

	require.NoError(t, err)
	require.NotNil(t, manager)
}
