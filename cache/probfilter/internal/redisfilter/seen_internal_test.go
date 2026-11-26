// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisfilter

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/alicebob/miniredis/v2/server"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/cache/probfilter"

	goredis "github.com/redis/go-redis/v9"
)

// TestCore_ReadyObservationMakesALostFilterAnError pins that a process that
// merely observed a committed rebuild (without ever looking a value up) also
// fails closed when the filter is lost afterwards.
func TestCore_ReadyObservationMakesALostFilterAnError(t *testing.T) {
	t.Parallel()

	observations := map[string]func(*testing.T, *Core){
		"RebuildCommitted": func(t *testing.T, c *Core) {
			ok, err := c.RebuildCommitted(t.Context())
			require.NoError(t, err)
			require.True(t, ok)
		},
		"LastRebuildCommit": func(t *testing.T, c *Core) {
			at, err := c.LastRebuildCommit(t.Context())
			require.NoError(t, err)
			require.False(t, at.IsZero())
		},
		"RebuildCommitAge": func(t *testing.T, c *Core) {
			_, _, known, err := c.RebuildCommitAge(t.Context())
			require.NoError(t, err)
			require.True(t, known)
		},
	}
	for name, observe := range observations {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			mr := miniredis.RunT(t)
			require.NoError(t, mr.Server().Register("T.EXISTS", func(c *server.Peer, _ string, _ []string) { c.WriteInt(0) }))
			client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
			t.Cleanup(func() { _ = client.Close() })
			core := New(client, "t:f", Commands{Label: "Test", Exists: "T.EXISTS"})

			require.NoError(t, mr.Set("t:f", "filter"))
			require.NoError(t, mr.Set(core.readyKey, "1700000000000"))
			observe(t, core)

			mr.Del("t:f")
			_, err := core.MightExist(t.Context(), "a")
			require.ErrorIs(t, err, probfilter.ErrFilterLost)
		})
	}
}
