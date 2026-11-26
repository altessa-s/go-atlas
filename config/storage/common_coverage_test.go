// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package storageconfig_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

func requireFieldError(t *testing.T, err error, field string) {
	t.Helper()
	verrs, ok := errors.AsType[validation.Errors](err)
	require.True(t, ok, "want validation.Errors, got %v", err)
	require.Contains(t, verrs, field)
}

func TestNATSConfig_Validate(t *testing.T) {
	t.Parallel()

	require.NoError(t, (&storageconfig.NATSConfig{Replicas: 1}).Validate())
	require.NoError(t, (&storageconfig.NATSConfig{Replicas: storageconfig.MaxNATSReplicas}).Validate())
	requireFieldError(t, (&storageconfig.NATSConfig{}).Validate(), "Replicas")
	requireFieldError(t, (&storageconfig.NATSConfig{Replicas: storageconfig.MaxNATSReplicas + 1}).Validate(), "Replicas")
}

func TestRedisConfig_Validate(t *testing.T) {
	t.Parallel()

	require.NoError(t, (&storageconfig.RedisConfig{}).Validate())
	require.NoError(t, (&storageconfig.RedisConfig{KeysPrefix: "svc:"}).Validate())
	requireFieldError(t, (&storageconfig.RedisConfig{KeysPrefix: "   "}).Validate(), "KeysPrefix")
	long := strings.Repeat("k", storageconfig.MaxKeyPrefixLength+1)
	requireFieldError(t, (&storageconfig.RedisConfig{KeysPrefix: long}).Validate(), "KeysPrefix")
}

func TestMemoryConfig_Validate(t *testing.T) {
	t.Parallel()

	require.NoError(t, (&storageconfig.MemoryConfig{}).Validate())
}

func TestBackendConfig_NormalizeAndValidate(t *testing.T) {
	t.Parallel()

	t.Run("normalize fills the selected sub-config only", func(t *testing.T) {
		t.Parallel()
		for _, typ := range []storageconfig.BackendType{
			storageconfig.BackendTypeMemory, storageconfig.BackendTypeNATS, storageconfig.BackendTypeRedis,
		} {
			c := storageconfig.BackendConfig{Type: typ}
			c.Normalize()
			require.Equal(t, typ == storageconfig.BackendTypeMemory, c.Memory != nil, typ)
			require.Equal(t, typ == storageconfig.BackendTypeNATS, c.NATS != nil, typ)
			require.Equal(t, typ == storageconfig.BackendTypeRedis, c.Redis != nil, typ)
		}
	})

	t.Run("normalize keeps an existing sub-config", func(t *testing.T) {
		t.Parallel()
		nats := &storageconfig.NATSConfig{Bucket: "b", Replicas: 2}
		c := storageconfig.BackendConfig{Type: storageconfig.BackendTypeNATS, NATS: nats}
		c.Normalize()
		require.Same(t, nats, c.NATS)
	})

	t.Run("normalize ignores unknown type", func(t *testing.T) {
		t.Parallel()
		c := storageconfig.BackendConfig{Type: "etcd"}
		c.Normalize()
		require.Nil(t, c.Memory)
		require.Nil(t, c.NATS)
		require.Nil(t, c.Redis)
	})

	t.Run("validate", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, (&storageconfig.BackendConfig{
			Type: storageconfig.BackendTypeRedis, Redis: &storageconfig.RedisConfig{KeysPrefix: "p"},
		}).Validate())
		requireFieldError(t, (&storageconfig.BackendConfig{}).Validate(), "Type")
		requireFieldError(t, (&storageconfig.BackendConfig{Type: "etcd"}).Validate(), "Type")
	})
}

func TestCacheStorageConfig_NormalizeAndValidate(t *testing.T) {
	t.Parallel()

	mem := storageconfig.CacheStorageConfig{Type: storageconfig.CacheStorageTypeMemory}
	mem.Normalize()
	require.NotNil(t, mem.Memory)
	require.Nil(t, mem.Redis)

	redis := storageconfig.CacheStorageConfig{Type: storageconfig.CacheStorageTypeRedis}
	redis.Normalize()
	require.NotNil(t, redis.Redis)
	require.Nil(t, redis.Memory)

	unknown := storageconfig.CacheStorageConfig{Type: "nats"}
	unknown.Normalize()
	require.Nil(t, unknown.Memory)
	require.Nil(t, unknown.Redis)

	require.NoError(t, redis.Validate())
	requireFieldError(t, unknown.Validate(), "Type")
	requireFieldError(t, (&storageconfig.CacheStorageConfig{}).Validate(), "Type")
}
