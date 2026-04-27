// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"

	"github.com/alicebob/miniredis/v2"

	idempredis "github.com/altessa-s/go-atlas/data/idempotency/storages/redis"
	goredis "github.com/redis/go-redis/v9"
)

func FuzzStorage_AttemptLock(f *testing.F) {
	f.Add("key1", []byte("value1"))
	f.Add("", []byte(""))
	f.Add("special:key", []byte("data"))

	mr, err := miniredis.Run()
	if err != nil {
		f.Fatalf("failed to start miniredis: %v", err)
	}
	f.Cleanup(func() { mr.Close() })

	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	storage := idempredis.New(client)
	ctx := f.Context()

	f.Fuzz(func(t *testing.T, key string, val []byte) {
		// Should not panic regardless of input
		_, _, _, _ = storage.AttemptLock(ctx, key, val)
		_ = storage.Complete(ctx, key, val, nil)
		_ = storage.Delete(ctx, key)
	})
}
