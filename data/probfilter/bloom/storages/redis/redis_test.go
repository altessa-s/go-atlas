// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	bfredis "github.com/altessa-s/go-atlas/data/probfilter/bloom/storages/redis"
	goredis "github.com/redis/go-redis/v9"
)

// Note: Redis Bloom filter commands (BF.*) require RedisBloom module.
// miniredis doesn't support these commands, so we test structural behavior only.

func TestNew(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })

	s := bfredis.New(client, "test-bloom")
	if s == nil {
		t.Fatal("New() returned nil")
	}
}

func TestNew_WithOptions(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })

	s := bfredis.New(client, "test-bloom",
		bfredis.WithKeyPrefix("custom:"),
		bfredis.WithExpectedItems(50000),
		bfredis.WithFalsePositiveRate(0.001),
	)
	if s == nil {
		t.Fatal("New() returned nil")
	}
}

func TestStorage_LastRebuild(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })

	s := bfredis.New(client, "test-bloom")
	if !s.LastRebuild().IsZero() {
		t.Error("LastRebuild() should be zero initially")
	}

	now := time.Now()
	s.SetLastRebuild(now)
	if !s.LastRebuild().Equal(now) {
		t.Errorf("LastRebuild() = %v, want %v", s.LastRebuild(), now)
	}
}

func TestStorage_Close(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })

	s := bfredis.New(client, "test-bloom")
	if err := s.Close(t.Context()); err != nil {
		t.Errorf("Close() error: %v", err)
	}
}
