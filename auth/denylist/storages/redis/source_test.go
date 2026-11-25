// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/alicebob/miniredis/v2/server"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/denylist/mirror"
	"github.com/altessa-s/go-atlas/auth/denylist/negcache"
	"github.com/altessa-s/go-atlas/auth/jwt"
	"github.com/altessa-s/go-atlas/cache/probfilter/bloom"
	"github.com/altessa-s/go-atlas/cache/probfilter/bloom/storages/memory"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/redistest"

	redisstore "github.com/altessa-s/go-atlas/auth/denylist/storages/redis"
)

// miniredis has no standalone topology or process ID. Tests can override a reply.
func serveScanTopology(mr *miniredis.Miniredis, override server.Hook) {
	mr.Server().SetPreHook(func(peer *server.Peer, cmd string, args ...string) bool {
		if override != nil && override(peer, cmd, args...) {
			return true
		}
		switch {
		case cmd == "ROLE":
			peer.WriteLen(1)
			peer.WriteBulk("master")
		case cmd == "INFO" && args[0] == "cluster":
			peer.WriteBulk("cluster_enabled:0\r\n")
		case cmd == "INFO" && args[0] == "server":
			peer.WriteBulk("run_id:primary-A\r\n")
		default:
			return false
		}
		return true
	})
}

func TestRawSourceAbortsOnDrift(t *testing.T) {
	t.Parallel()
	for _, drift := range []string{"role", "cluster", "run_id", "missing run_id"} {
		t.Run(drift, func(t *testing.T) {
			t.Parallel()
			client, mr := redistest.RedisClient(t)
			store, err := redisstore.New(client, redisstore.WithKeyPrefix("custom:"))
			require.NoError(t, err)
			var pages atomic.Int32
			var firstPeer atomic.Pointer[server.Peer]
			var changedPeer atomic.Bool
			serveScanTopology(mr, func(p *server.Peer, cmd string, args ...string) bool {
				switch cmd {
				case "ROLE", "INFO", "SCAN":
					firstPeer.CompareAndSwap(nil, p)
					if firstPeer.Load() != p {
						changedPeer.Store(true)
					}
				}
				if cmd == "SCAN" {
					n := pages.Add(1)
					p.WriteLen(2)
					if n == 1 {
						p.WriteBulk("1")
					} else {
						p.WriteBulk("0")
					}
					p.WriteLen(1)
					if n == 1 {
						p.WriteBulk("custom:first")
					} else {
						p.WriteBulk("custom:discard")
					}
					return true
				}
				if pages.Load() == 2 {
					switch {
					case drift == "role" && cmd == "ROLE":
						p.WriteLen(1)
						p.WriteBulk("slave")
					case drift == "cluster" && cmd == "INFO" && args[0] == "cluster":
						p.WriteBulk("cluster_enabled:1\r\n")
					case drift == "run_id" && cmd == "INFO" && args[0] == "server":
						p.WriteBulk("run_id:primary-B\r\n")
					case drift == "missing run_id" && cmd == "INFO" && args[0] == "server":
						p.WriteBulk("# Server\r\n")
					default:
						return false
					}
					return true
				}
				return false
			})
			var keys []string
			var failures []error
			for k, err := range store.RawSource().StreamValues(t.Context()) {
				if err != nil {
					require.Empty(t, k)
					failures = append(failures, err)
					continue
				}
				keys = append(keys, k)
				// Force ordinary pooled calls to use another connection while this cursor is open.
				require.NoError(t, client.Ping(t.Context()).Err())
			}
			require.Equal(t, []string{"first"}, keys)
			require.Len(t, failures, 1)
			require.ErrorIs(t, failures[0], redisstore.ErrUnsupportedClient)
			require.EqualValues(t, 2, pages.Load())
			require.False(t, changedPeer.Load())
		})
	}
}

func TestRawSourceKeepsPinnedConnectionUntilIteratorStops(t *testing.T) {
	t.Parallel()
	client, mr := redistest.RedisClient(t)
	serveScanTopology(mr, nil)
	store, err := redisstore.New(client, redisstore.WithKeyPrefix("custom:"))
	require.NoError(t, err)
	require.NoError(t, store.Revoke(t.Context(), "raw-jti"))
	for key, err := range store.RawSource().StreamValues(t.Context()) {
		require.NoError(t, err)
		require.Equal(t, "raw-jti", key)
		require.NoError(t, client.Ping(t.Context()).Err())
		require.EqualValues(t, 2, client.PoolStats().TotalConns)
		break
	}
	require.Equal(t, client.PoolStats().TotalConns, client.PoolStats().IdleConns)
	m := mirror.New(store.RawSource())
	require.NoError(t, m.Refresh(t.Context()))
	verifier := jwt.NewVerifier(nil, jwt.WithRevocation(m))
	require.ErrorIs(t, verifier.ValidateClaims(jwt.Claims{"jti": "raw-jti", "exp": time.Now().Add(time.Hour).Unix()}), jwt.ErrTokenRevoked)
	cache := negcache.New(bloom.New(memory.New()), store)
	require.NoError(t, cache.Rebuild(t.Context(), store.DataLoader()))
	revoked, err := cache.IsRevoked(t.Context(), "raw-jti")
	require.NoError(t, err)
	require.True(t, revoked)
	require.NoError(t, cache.Close(t.Context()))
	require.NoError(t, m.Refresh(t.Context()))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for _, err := range store.RawSource().StreamValues(ctx) {
		require.ErrorIs(t, err, context.Canceled)
	}
}
