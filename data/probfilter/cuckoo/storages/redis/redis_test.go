// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	cfredis "github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages/redis"
)

// Note: Redis Cuckoo filter commands (CF.*) require RedisBloom module.
// miniredis doesn't support these commands, so we test structural behavior only.

func TestNew(t *testing.T) {
	client, _ := testhelpers.RedisClient(t)

	s := cfredis.New(client, "test-filter")
	require.NotNil(t, s, "New() returned nil")
}

func TestNew_WithOptions(t *testing.T) {
	client, _ := testhelpers.RedisClient(t)

	s := cfredis.New(client, "test-filter",
		cfredis.WithKeyPrefix("custom:"),
		cfredis.WithCapacity(50000),
	)
	require.NotNil(t, s, "New() returned nil")
}

func TestStorage_Close(t *testing.T) {
	client, _ := testhelpers.RedisClient(t)

	s := cfredis.New(client, "test-filter")
	require.NoError(t, s.Close(t.Context()))
}
