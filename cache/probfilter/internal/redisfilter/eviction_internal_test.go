// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisfilter_test

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2/server"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/cache/probfilter/internal/redisfilter"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/redistest"
)

// Only a denied CONFIG on a verified primary is eligible for the managed
// service exception. Empty replies and ordinary command failures are errors.
func TestConfigPermissionRequiredForUnverifiedResult(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"denied", "empty", "error", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			client, mr := redistest.RedisClient(t)
			redistest.ServeTopology(mr, "master", "0")
			require.NoError(t, mr.Server().Register("CONFIG", func(c *server.Peer, _ string, _ []string) {
				switch mode {
				case "denied":
					c.WriteError("NOPERM configuration disabled")
				case "empty":
					c.WriteMapLen(0)
				default:
					c.WriteError("ERR unexpected")
				}
			}))
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if mode == "canceled" {
				cancel()
			}
			verified, err := redisfilter.CheckEvictionPolicy(ctx, client)
			require.False(t, verified)
			if mode == "denied" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			if mode == "canceled" {
				require.ErrorIs(t, err, context.Canceled)
			}
		})
	}
}
