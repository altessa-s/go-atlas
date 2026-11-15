// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
	"github.com/altessa-s/go-atlas/store/mongo"

	cursnats "github.com/altessa-s/go-atlas/store/mongo/storages/nats"
)

func FuzzStorage_StoreLoad(f *testing.F) {
	f.Add("key1", "cursor1", "sort1")
	f.Add("special.key", "c", "s")

	ns := natstest.StartNATSServer(f)
	_, js := natstest.ConnectJetStream(f, ns)
	kv := natstest.CreateNATSKV(f, js, "fuzz-cursor", time.Hour)
	ctx := f.Context()
	s, err := cursnats.New(ctx, kv)
	require.NoError(f, err)

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
	})
}
