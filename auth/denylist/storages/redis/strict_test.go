// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"sync/atomic"
	"testing"

	"github.com/alicebob/miniredis/v2/server"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/redistest"

	redisstore "github.com/altessa-s/go-atlas/auth/denylist/storages/redis"
	goredis "github.com/redis/go-redis/v9"
)

func TestStrictClientAdmission(t *testing.T) {
	t.Parallel()
	single, mr := redistest.RedisClient(t)
	for _, tc := range []struct {
		name   string
		client goredis.UniversalClient
		want   error
	}{
		{"single", single, nil},
		{"primary cluster", goredis.NewClusterClient(&goredis.ClusterOptions{}), nil},
		{"ring", goredis.NewRing(&goredis.RingOptions{}), redisstore.ErrUnsupportedClient},
		{"decorator", struct{ goredis.UniversalClient }{single}, redisstore.ErrUnsupportedClient},
		{"read only", goredis.NewClusterClient(&goredis.ClusterOptions{ReadOnly: true}), redisstore.ErrReplicaReadNotAllowed},
		{"latency", goredis.NewClusterClient(&goredis.ClusterOptions{RouteByLatency: true}), redisstore.ErrReplicaReadNotAllowed},
		{"random", goredis.NewClusterClient(&goredis.ClusterOptions{RouteRandomly: true}), redisstore.ErrReplicaReadNotAllowed},
		{"configured cache", goredis.NewClient(&goredis.Options{ClientSideCacheConfig: &goredis.ClientSideCacheConfig{}}), redisstore.ErrUnsupportedClient},
		{"custom cache", goredis.NewClient(&goredis.Options{ClientSideCache: goredis.NewLocalCache(goredis.CacheConfig{})}), redisstore.ErrUnsupportedClient},
		{"cluster cache", goredis.NewClusterClient(&goredis.ClusterOptions{ClientSideCacheConfig: &goredis.ClientSideCacheConfig{}}), redisstore.ErrUnsupportedClient},
		{"custom node", goredis.NewClusterClient(&goredis.ClusterOptions{NewClient: func(o *goredis.Options) *goredis.Client { return goredis.NewClient(o) }}), redisstore.ErrUnsupportedClient},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.client != single && tc.name != "decorator" {
				t.Cleanup(func() { _ = tc.client.Close() })
			}
			_, err := redisstore.New(tc.client)
			require.NoError(t, err)
			store, err := redisstore.New(tc.client, redisstore.WithStrictExactReads())
			require.ErrorIs(t, err, tc.want)
			if tc.want != nil {
				require.Nil(t, store)
			}
			require.Zero(t, mr.CommandCount())
		})
	}
}

func TestStrictReadVerifiesRoleAndExistsInOneScript(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, info string
		denied     bool
	}{
		{"primary", "# Replication\r\nrole:master\r\n", false},
		{"replica", "role:slave\r\n", true},
		{"wrong field", "previous_role:master\r\n", true},
		{"wrong value", "role:master_candidate\r\n", true},
		{"empty", "", true},
		{"info denied", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client, mr := redistest.RedisClient(t)
			var calls atomic.Int32
			mr.Server().SetPreHook(func(p *server.Peer, cmd string, args ...string) bool {
				switch {
				case cmd == "EVAL_RO":
					calls.Add(1)
					// miniredis labels INFO as a write. Execute the actual Lua body through EVAL
					// so the test can supply replication INFO just as Redis does for EVAL_RO.
					mr.Server().Dispatch(p, append([]string{"EVAL"}, args...))
				case cmd == "INFO" && args[0] == "replication":
					if tc.name == "info denied" {
						p.WriteError("NOPERM")
					} else {
						p.WriteBulk(tc.info)
					}
				default:
					return false
				}
				return true
			})
			store, err := redisstore.New(client, redisstore.WithStrictExactReads())
			require.NoError(t, err)
			require.NoError(t, store.Revoke(t.Context(), "revoked"))
			for _, key := range []string{"revoked", "fresh"} {
				got, err := store.IsRevoked(t.Context(), key)
				if tc.denied {
					require.ErrorIs(t, err, redisstore.ErrReplicaReadNotAllowed)
					require.False(t, got)
				} else {
					require.NoError(t, err)
					require.Equal(t, key == "revoked", got)
				}
			}
			require.EqualValues(t, 2, calls.Load())
		})
	}
}
