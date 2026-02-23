// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisutils_test

import (
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/altessa-s/go-atlas/data/internal/redisutils"

	goredis "github.com/redis/go-redis/v9"
)

func setupRedis(tb testing.TB) (*goredis.Client, *miniredis.Miniredis) {
	tb.Helper()
	mr := miniredis.RunT(tb)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	return client, mr
}

func TestGetBytes_Hit(t *testing.T) {
	client, mr := setupRedis(t)
	defer mr.Close()
	ctx := t.Context()

	mr.Set("key1", "value1")

	got, err := redisutils.GetBytes(ctx, client, "key1", errors.New("not found"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "value1" {
		t.Errorf("GetBytes() = %q, want %q", got, "value1")
	}
}

func TestGetBytes_Miss(t *testing.T) {
	client, mr := setupRedis(t)
	defer mr.Close()
	ctx := t.Context()

	notFoundErr := errors.New("not found")
	_, err := redisutils.GetBytes(ctx, client, "missing", notFoundErr)
	if !errors.Is(err, notFoundErr) {
		t.Errorf("GetBytes() error = %v, want %v", err, notFoundErr)
	}
}

func TestSetBytes(t *testing.T) {
	client, mr := setupRedis(t)
	defer mr.Close()
	ctx := t.Context()

	err := redisutils.SetBytes(ctx, client, "key1", []byte("val"), 0)
	if err != nil {
		t.Fatalf("SetBytes() error: %v", err)
	}

	got, err := mr.Get("key1")
	if err != nil {
		t.Fatalf("miniredis Get error: %v", err)
	}
	if got != "val" {
		t.Errorf("stored value = %q, want %q", got, "val")
	}
}

func TestSetBytes_WithTTL(t *testing.T) {
	client, mr := setupRedis(t)
	defer mr.Close()
	ctx := t.Context()

	err := redisutils.SetBytes(ctx, client, "key1", []byte("val"), 10*time.Second)
	if err != nil {
		t.Fatalf("SetBytes() error: %v", err)
	}

	mr.FastForward(15 * time.Second)

	if mr.Exists("key1") {
		t.Error("key should have expired")
	}
}

func TestDel(t *testing.T) {
	client, mr := setupRedis(t)
	defer mr.Close()
	ctx := t.Context()

	mr.Set("key1", "value1")

	err := redisutils.Del(ctx, client, "key1")
	if err != nil {
		t.Fatalf("Del() error: %v", err)
	}

	if mr.Exists("key1") {
		t.Error("key should have been deleted")
	}
}

func TestDel_NonExistent(t *testing.T) {
	client, mr := setupRedis(t)
	defer mr.Close()
	ctx := t.Context()

	err := redisutils.Del(ctx, client, "missing")
	if err != nil {
		t.Fatalf("Del() on non-existent key should not error: %v", err)
	}
}
