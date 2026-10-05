// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisfilter_test

import (
	"fmt"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/alicebob/miniredis/v2/server"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/probfilter/internal/redisfilter"

	goredis "github.com/redis/go-redis/v9"
)

// writeBool answers like RedisBloom: an integer under RESP2, a boolean under
// RESP3.
func writeBool(c *server.Peer, v bool) {
	switch {
	case c.Resp3 && v:
		c.WriteRaw("#t\r\n")
	case c.Resp3:
		c.WriteRaw("#f\r\n")
	case v:
		c.WriteInt(1)
	default:
		c.WriteInt(0)
	}
}

func protocolClient(t *testing.T, mr *miniredis.Miniredis, protocol int) *goredis.Client {
	t.Helper()
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr(), Protocol: protocol})
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestCore_MightExist_BothProtocols(t *testing.T) {
	t.Parallel()

	for _, protocol := range []int{2, 3} {
		t.Run(fmt.Sprintf("RESP%d", protocol), func(t *testing.T) {
			t.Parallel()
			mr := miniredis.RunT(t)
			require.NoError(t, mr.Server().Register(testCommands.Exists, func(c *server.Peer, _ string, args []string) {
				writeBool(c, args[1] == "present")
			}))
			core := redisfilter.New(protocolClient(t, mr, protocol), "t:f", testCommands)

			ok, err := core.MightExist(t.Context(), "present")
			require.NoError(t, err)
			require.True(t, ok)

			ok, err = core.MightExist(t.Context(), "absent")
			require.NoError(t, err)
			require.False(t, ok)
		})
	}
}

func TestCore_Info_BothProtocols(t *testing.T) {
	t.Parallel()

	for _, protocol := range []int{2, 3} {
		t.Run(fmt.Sprintf("RESP%d", protocol), func(t *testing.T) {
			t.Parallel()
			mr := miniredis.RunT(t)
			require.NoError(t, mr.Server().Register(testCommands.Info, func(c *server.Peer, _ string, _ []string) {
				if c.Resp3 {
					c.WriteMapLen(2)
				} else {
					c.WriteLen(4)
				}
				c.WriteBulk("Capacity")
				c.WriteInt(100)
				c.WriteBulk("Number of items inserted")
				c.WriteInt(42)
			}))
			core := redisfilter.New(protocolClient(t, mr, protocol), "t:f", testCommands)

			result, found, err := core.Info(t.Context())
			require.NoError(t, err)
			require.True(t, found)

			fields := map[string]int64{}
			for key, raw := range redisfilter.InfoFields(result) {
				val, convErr := redisfilter.ToInt64(raw)
				require.NoError(t, convErr)
				fields[key] = val
			}
			require.Equal(t, map[string]int64{"Capacity": 100, "Number of items inserted": 42}, fields)
		})
	}
}

func TestToBool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   any
		want    bool
		wantErr bool
	}{
		{name: "RESP3 true", input: true, want: true},
		{name: "RESP3 false", input: false, want: false},
		{name: "RESP2 one", input: int64(1), want: true},
		{name: "RESP2 zero", input: int64(0), want: false},
		{name: "int", input: 1, want: true},
		{name: "numeric string", input: "1", want: true},
		{name: "bad string", input: "yes", wantErr: true},
		{name: "unsupported type", input: 1.0, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := redisfilter.ToBool(tc.input)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
