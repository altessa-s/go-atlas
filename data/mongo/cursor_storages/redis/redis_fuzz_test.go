// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/mongo"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	cursredis "github.com/altessa-s/go-atlas/data/mongo/cursor_storages/redis"
)

func FuzzStorage_StoreLoad(f *testing.F) {
	f.Add("key1", "cursor1", "sort1")
	f.Add("special-key", "c", "s")

	client, _ := testhelpers.RedisClient(f)
	s := cursredis.New(client)
	ctx := f.Context()

	f.Fuzz(func(t *testing.T, key, cursorId, sort string) {
		if key == "" {
			return
		}
		meta := &mongo.CursorMetadata{
			CursorId:      cursorId,
			Sort:          sort,
			CursorIdField: "_id",
			FilterHash:    "hash",
			CreatedAt:     time.Now(),
		}
		// Should not panic
		_ = s.Store(ctx, key, meta)
		_, _ = s.Load(ctx, key)
		_ = s.Delete(ctx, key)
	})
}
