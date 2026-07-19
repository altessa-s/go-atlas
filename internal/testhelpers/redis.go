// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package testhelpers

import (
	"testing"

	"github.com/alicebob/miniredis/v2"

	goredis "github.com/redis/go-redis/v9"
)

// RedisClient starts an in-process miniredis server and returns a go-redis
// client connected to it, together with the server handle for direct data
// manipulation (seeding keys, FastForward, FlushAll). Both the client and
// the server are shut down automatically via tb.Cleanup.
func RedisClient(tb testing.TB) (*goredis.Client, *miniredis.Miniredis) {
	tb.Helper()

	mr := miniredis.RunT(tb)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	tb.Cleanup(func() { _ = client.Close() })
	return client, mr
}
