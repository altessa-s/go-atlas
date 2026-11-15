// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/store/mongo/storages/redis"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/redistest"
	"github.com/altessa-s/go-atlas/store/mongo"

	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	mongofactory "github.com/altessa-s/go-atlas/factory/store/mongo"
)

func TestBuilder_UsesClientPrefixAndTTL(t *testing.T) {
	t.Parallel()

	client, mr := redistest.RedisClient(t)
	cfg := &storageconfig.BackendConfig{
		Type:  storageconfig.BackendTypeRedis,
		Redis: &storageconfig.RedisConfig{KeysPrefix: "cur:"},
	}

	s, err := mongofactory.New(cfg).
		UseDefaultLogger().
		UseStorage(storageconfig.BackendTypeRedis, factory.Builder(client)).
		WithTTL(time.Minute).
		Build(t.Context())
	require.NoError(t, err)

	meta := &mongo.CursorMetadata{CursorId: "c1", Sort: "name"}
	require.NoError(t, s.Store(t.Context(), "key-1", meta))
	got, err := s.Load(t.Context(), "key-1")
	require.NoError(t, err)
	require.Equal(t, "c1", got.CursorId)

	keys := mr.Keys()
	require.Len(t, keys, 1)
	require.Contains(t, keys[0], "cur:")
	require.Equal(t, time.Minute, mr.TTL(keys[0]))
}

func TestBuilder_Errors(t *testing.T) {
	t.Parallel()

	_, err := factory.Builder(nil)(t.Context(), &storageconfig.BackendConfig{
		Type: storageconfig.BackendTypeRedis, Redis: &storageconfig.RedisConfig{},
	}, 0, nil)
	require.ErrorIs(t, err, factory.ErrClientRequired)

	client, _ := redistest.RedisClient(t)
	_, err = factory.Builder(client)(t.Context(), &storageconfig.BackendConfig{Type: storageconfig.BackendTypeRedis}, 0, nil)
	require.ErrorIs(t, err, factory.ErrConfigRequired)
}
