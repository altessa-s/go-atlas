// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"

	"github.com/alicebob/miniredis/v2"

	uniqredis "github.com/altessa-s/go-atlas/data/uniq/providers/redis"
	goredis "github.com/redis/go-redis/v9"
)

func FuzzProvider_AddExist(f *testing.F) {
	f.Add("key1", "value1")
	f.Add("special-key", "data")

	mr := miniredis.RunT(f)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	f.Cleanup(func() { client.Close() })
	p := uniqredis.New(client)
	ctx := f.Context()

	f.Fuzz(func(t *testing.T, key, value string) {
		if key == "" {
			return
		}
		// Should not panic
		_ = p.AddWithValue(ctx, key, []byte(value))
		_, _ = p.Exist(ctx, key)
		_, _ = p.GetValue(ctx, key)
	})
}
