// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kvstore_test

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/mongo"
	"github.com/altessa-s/go-atlas/data/mongo/cursor_storages/kvstore"
)

func FuzzJSONStorage_StoreLoad(f *testing.F) {
	f.Add("key1", "cursor123", "sort-val", "_id", "hash1")
	f.Add("", "c", "s", "f", "h")

	backend := newMockBackend()
	s := kvstore.NewJSONStorage(backend, "fuzz")
	ctx := f.Context()

	f.Fuzz(func(t *testing.T, key, cursorId, sort, field, hash string) {
		if key == "" {
			return
		}
		meta := &mongo.CursorMetadata{
			CursorId:      cursorId,
			Sort:          sort,
			CursorIdField: field,
			FilterHash:    hash,
			CreatedAt:     time.Now(),
		}
		// Should not panic
		_ = s.Store(ctx, key, meta)
		_, _ = s.Load(ctx, key)
		_ = s.Delete(ctx, key)
	})
}
