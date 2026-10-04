// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/idempotency"
	"github.com/altessa-s/go-atlas/data/idempotency/storages"
	"github.com/altessa-s/go-atlas/data/idempotency/storages/memory"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	idnats "github.com/altessa-s/go-atlas/data/idempotency/storages/nats"
	idredis "github.com/altessa-s/go-atlas/data/idempotency/storages/redis"
)

func BenchmarkRelease(b *testing.B) {
	for _, backend := range []string{"memory", "redis", "nats"} {
		b.Run(backend, func(b *testing.B) {
			var store storages.Storage
			switch backend {
			case "memory":
				store = memory.New()
			case "redis":
				client, _ := testhelpers.RedisClient(b)
				store = idredis.New(client)
			case "nats":
				server := testhelpers.StartNATSServer(b)
				_, js := testhelpers.ConnectJetStream(b, server)
				var err error
				store, err = idnats.New(js, idnats.WithBucket("release_bench"))
				require.NoError(b, err)
			}
			keeper := idempotency.New(store, idempotency.WithMaxLockDuration(idempotency.DefaultMaxLockDuration))
			for b.Loop() {
				b.StopTimer()
				locked, state, err := keeper.AttemptLock(b.Context(), "key")
				require.NoError(b, err)
				require.True(b, locked)
				b.StartTimer()
				require.NoError(b, keeper.Release(b.Context(), "key", state))
			}
		})
	}
}
