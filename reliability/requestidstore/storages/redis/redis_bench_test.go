// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-aip/requestid"
	"github.com/altessa-s/go-aip/requestid/requestidtest"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/redistest"
	"github.com/altessa-s/go-atlas/reliability/requestidstore/storages/redis"
)

func BenchmarkReserveLive(b *testing.B) {
	client, _ := redistest.RedisClient(b)
	store := redis.New(client)
	now := requestidtest.Base
	key := requestidtest.Key(b.Name())
	entry := requestid.Entry{State: requestid.StatePending, Token: "owner", ExpiresAt: now.Add(time.Hour)}
	_, _, err := store.Reserve(b.Context(), key, entry, now)
	require.NoError(b, err)
	for b.Loop() {
		_, _, err = store.Reserve(b.Context(), key, entry, now)
		require.NoError(b, err)
	}
}
