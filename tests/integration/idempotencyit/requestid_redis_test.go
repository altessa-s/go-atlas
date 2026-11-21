// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotencyit_test

import (
	"testing"

	"github.com/altessa-s/go-aip/requestid"
	"github.com/altessa-s/go-aip/requestid/requestidtest"

	"github.com/altessa-s/go-atlas/reliability/requestidstore/storages/redis"
	"github.com/altessa-s/go-atlas/tests/integration/internal/livebackend"
)

func TestRequestIDRedis(t *testing.T) {
	t.Parallel()
	requestidtest.Run(t, func(t *testing.T) requestid.Store {
		client := livebackend.ConnectRedis(t)
		return redis.New(client, redis.WithKeyPrefix(livebackend.RedisPrefix(t, client, "requestid")))
	})
}
