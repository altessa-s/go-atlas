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

// TestStorage_Stats_ParsesCFInfo replays CF.INFO replies of real RedisBloom
// filters: Capacity counts fingerprint slots over every sub-filter, while
// "Size" is the memory footprint in bytes and lands in MemoryUsageBytes.
func TestStorage_Stats_ParsesCFInfo(t *testing.T) {
	t.Parallel()

	type field struct {
		key string
		val any // int, or a string for a malformed value
	}
	tests := []struct {
		name         string
		info         []field
		wantCapacity int64
		wantItems    int64
		wantBytes    int64
	}{
		{
			// CF.RESERVE k 1000
			name: "one filter",
			info: []field{
				{"Size", 1080}, {"Number of buckets", 512}, {"Number of filters", 1},
				{"Number of items inserted", 256}, {"Number of items deleted", 0},
				{"Bucket size", 2}, {"Expansion rate", 1}, {"Max iterations", 20},
			},
			wantCapacity: 1024,
			wantItems:    256,
			wantBytes:    1080,
		},
		{
			// CF.RESERVE k 100 EXPANSION 2, then 400 inserts: 64+128+256 buckets.
			name: "expanded twice",
			info: []field{
				{"Size", 984}, {"Number of buckets", 64}, {"Number of filters", 3},
				{"Number of items inserted", 400}, {"Number of items deleted", 0},
				{"Bucket size", 2}, {"Expansion rate", 2}, {"Max iterations", 20},
			},
			wantCapacity: 896,
			wantItems:    400,
			wantBytes:    984,
		},
		{
			// Expansion rate 1 adds sub-filters of the first one's size.
			name: "expansion rate one",
			info: []field{
				{"Size", 4176}, {"Number of buckets", 512}, {"Number of filters", 2},
				{"Number of items inserted", 1500}, {"Bucket size", 4}, {"Expansion rate", 1},
			},
			wantCapacity: 4096,
			wantItems:    1500,
			wantBytes:    4176,
		},
		{
			// CF.RESERVE k 1000 EXPANSION 0: the filter never grows.
			name: "expansion rate zero",
			info: []field{
				{"Size", 1080}, {"Number of buckets", 512}, {"Number of filters", 1},
				{"Number of items inserted", 10}, {"Bucket size", 2}, {"Expansion rate", 0},
			},
			wantCapacity: 1024,
			wantItems:    10,
			wantBytes:    1080,
		},
		{
			name:         "bucket fields missing",
			info:         []field{{"Size", 1080}, {"Number of items inserted", 3}},
			wantCapacity: 0,
			wantItems:    3,
			wantBytes:    1080,
		},
		{
			name: "malformed bucket count",
			info: []field{
				{"Size", 1080}, {"Number of buckets", "many"}, {"Number of filters", 1},
				{"Number of items inserted", 3}, {"Bucket size", 2},
			},
			wantCapacity: 0,
			wantItems:    3,
			wantBytes:    1080,
		},
		{
			name: "slot count overflows",
			info: []field{
				{"Size", 1080}, {"Number of buckets", 1 << 40}, {"Number of filters", 40},
				{"Number of items inserted", 3}, {"Bucket size", 2}, {"Expansion rate", 4},
			},
			wantCapacity: 0,
			wantItems:    3,
			wantBytes:    1080,
		},
	}

	for _, tc := range tests {
		for _, protocol := range []int{2, 3} {
			t.Run(fmt.Sprintf("%s/RESP%d", tc.name, protocol), func(t *testing.T) {
				t.Parallel()
				mr := miniredis.RunT(t)
				require.NoError(t, mr.Server().Register("CF.INFO", func(c *server.Peer, _ string, _ []string) {
					c.WriteMapLen(len(tc.info))
					for _, f := range tc.info {
						c.WriteBulk(f.key)
						switch v := f.val.(type) {
						case int:
							c.WriteInt(v)
						case string:
							c.WriteBulk(v)
						}
					}
				}))
				client := goredis.NewClient(&goredis.Options{Addr: mr.Addr(), Protocol: protocol})
				t.Cleanup(func() { _ = client.Close() })

				fs, err := cfredis.New(client, "f").Stats(t.Context())
				require.NoError(t, err)
				require.Equal(t, tc.wantCapacity, fs.Capacity)
				require.Equal(t, tc.wantItems, fs.ItemCount)
				require.Equal(t, tc.wantBytes, fs.MemoryUsageBytes)
				if tc.wantCapacity > 0 {
					require.InDelta(t, float64(tc.wantItems)/float64(tc.wantCapacity), fs.FillRatio, 1e-12)
				} else {
					require.Zero(t, fs.FillRatio)
				}
				require.Equal(t, "redis", fs.StorageType)
			})
		}
	}
}
