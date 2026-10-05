// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"fmt"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/alicebob/miniredis/v2/server"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	cfredis "github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages/redis"
	goredis "github.com/redis/go-redis/v9"
)

// Note: Redis Cuckoo filter commands (CF.*) require RedisBloom module.
// miniredis doesn't support these commands, so we test structural behavior only.

func TestNew(t *testing.T) {
	client, _ := testhelpers.RedisClient(t)

	s := cfredis.New(client, "test-filter")
	require.NotNil(t, s, "New() returned nil")
}

func TestNew_WithOptions(t *testing.T) {
	client, _ := testhelpers.RedisClient(t)

	s := cfredis.New(client, "test-filter",
		cfredis.WithKeyPrefix("custom:"),
		cfredis.WithCapacity(50000),
	)
	require.NotNil(t, s, "New() returned nil")
}

func TestStorage_Close(t *testing.T) {
	client, _ := testhelpers.RedisClient(t)

	s := cfredis.New(client, "test-filter")
	require.NoError(t, s.Close(t.Context()))
}

func TestStorage_Delete_BothProtocols(t *testing.T) {
	t.Parallel()

	for _, protocol := range []int{2, 3} {
		t.Run(fmt.Sprintf("RESP%d", protocol), func(t *testing.T) {
			t.Parallel()
			mr := miniredis.RunT(t)
			// CF.DEL replies like RedisBloom: 0/1 under RESP2, a boolean under RESP3.
			require.NoError(t, mr.Server().Register("CF.DEL", func(c *server.Peer, _ string, args []string) {
				deleted := args[1] == "present"
				switch {
				case c.Resp3 && deleted:
					c.WriteRaw("#t\r\n")
				case c.Resp3:
					c.WriteRaw("#f\r\n")
				case deleted:
					c.WriteInt(1)
				default:
					c.WriteInt(0)
				}
			}))
			client := goredis.NewClient(&goredis.Options{Addr: mr.Addr(), Protocol: protocol})
			t.Cleanup(func() { _ = client.Close() })
			s := cfredis.New(client, "f")

			deleted, err := s.Delete(t.Context(), "present")
			require.NoError(t, err)
			require.True(t, deleted)

			deleted, err = s.Delete(t.Context(), "absent")
			require.NoError(t, err)
			require.False(t, deleted)
		})
	}
}
