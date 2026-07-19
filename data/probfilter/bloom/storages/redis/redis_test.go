// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	bfredis "github.com/altessa-s/go-atlas/data/probfilter/bloom/storages/redis"
)

// Note: Redis Bloom filter commands (BF.*) require RedisBloom module.
// miniredis doesn't support these commands, so we test structural behavior only.

func TestNew(t *testing.T) {
	client, _ := testhelpers.RedisClient(t)

	s := bfredis.New(client, "test-bloom")
	require.NotNil(t, s, "New() returned nil")
}

func TestNew_WithOptions(t *testing.T) {
	client, _ := testhelpers.RedisClient(t)

	s := bfredis.New(client, "test-bloom",
		bfredis.WithKeyPrefix("custom:"),
		bfredis.WithExpectedItems(50000),
		bfredis.WithFalsePositiveRate(0.001),
	)
	require.NotNil(t, s, "New() returned nil")
}

func TestStorage_LastRebuild(t *testing.T) {
	client, _ := testhelpers.RedisClient(t)

	s := bfredis.New(client, "test-bloom")
	require.True(t, s.LastRebuild().IsZero(), "LastRebuild() should be zero initially")

	now := time.Now()
	s.SetLastRebuild(now)
	require.True(t, s.LastRebuild().Equal(now), "LastRebuild() = %v, want %v", s.LastRebuild(), now)
}

func TestStorage_Close(t *testing.T) {
	client, _ := testhelpers.RedisClient(t)

	s := bfredis.New(client, "test-bloom")
	require.NoError(t, s.Close(t.Context()))
}
