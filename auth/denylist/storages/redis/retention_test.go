// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2/server"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/redistest"

	redisstore "github.com/altessa-s/go-atlas/auth/denylist/storages/redis"
	goredis "github.com/redis/go-redis/v9"
)

func TestRevokeUntilRoundedMilliseconds(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		ttl time.Duration
		ms  string
	}{
		{time.Nanosecond, "1"}, {1900 * time.Microsecond, "2"}, {2 * time.Millisecond, "2"}, {math.MaxInt64, "9223372036855"},
	} {
		t.Run(tc.ttl.String(), func(t *testing.T) {
			t.Parallel()
			store, mr := setupStore(t)
			// Capture the write at the Redis boundary. The largest rounded value exceeds Go's Duration.
			sent := make(chan string, 1)
			mr.Server().SetPreHook(func(p *server.Peer, cmd string, args ...string) bool {
				if cmd != "SET" {
					return false
				}
				if len(args) == 4 {
					sent <- args[3]
				} else {
					sent <- ""
				}
				p.WriteOK()
				return true
			})
			require.NoError(t, store.RevokeUntil(t.Context(), "key", tc.ttl))
			require.Len(t, sent, 1)
			require.Equal(t, tc.ms, <-sent)
		})
	}
}

func TestRedisRevocationsOnlyExtend(t *testing.T) {
	t.Parallel()
	store, mr := setupStore(t)
	ctx := t.Context()
	for _, key := range []string{"long-short", "short-long", "bounded-permanent"} {
		require.NoError(t, store.RevokeUntil(ctx, key, time.Minute))
	}
	require.NoError(t, store.RevokeUntil(ctx, "long-short", time.Second))
	require.NoError(t, store.RevokeUntil(ctx, "short-long", time.Hour))
	require.NoError(t, store.Revoke(ctx, "bounded-permanent"))
	require.NoError(t, store.Revoke(ctx, "permanent-bounded"))
	require.NoError(t, store.RevokeUntil(ctx, "permanent-bounded", time.Minute))
	done := make(chan error, 1)
	go func() { done <- store.Revoke(ctx, "concurrent") }()
	require.NoError(t, store.RevokeUntil(ctx, "concurrent", time.Minute))
	require.NoError(t, <-done)
	mr.FastForward(2 * time.Second)
	require.True(t, mr.Exists(redisstore.DefaultKeyPrefix+"long-short"))
	mr.FastForward(time.Minute)
	require.True(t, mr.Exists(redisstore.DefaultKeyPrefix+"short-long"))
	mr.FastForward(time.Hour)
	for _, key := range []string{"bounded-permanent", "permanent-bounded", "concurrent"} {
		require.True(t, mr.Exists(redisstore.DefaultKeyPrefix+key), key)
		require.NoError(t, store.Restore(ctx, key))
		require.False(t, mr.Exists(redisstore.DefaultKeyPrefix+key))
	}
}

type commandFailure struct {
	command string
	cause   error
}

func (h commandFailure) DialHook(next goredis.DialHook) goredis.DialHook { return next }
func (h commandFailure) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return next
}
func (h commandFailure) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return func(ctx context.Context, cmd goredis.Cmder) error {
		if cmd.Name() == h.command {
			return h.cause
		}
		return next(ctx, cmd)
	}
}

func TestStoreErrorsHideCredentialsAndMatchCause(t *testing.T) {
	t.Parallel()
	const key = "private-jti"
	const secret = "private-password"
	for _, op := range []string{"exists", "set", "eval", "del", "role", "scan", "eval_ro"} {
		t.Run(op, func(t *testing.T) {
			t.Parallel()
			client, mr := redistest.RedisClient(t)
			serveScanTopology(mr, nil)
			cause := errors.New(secret)
			client.AddHook(commandFailure{command: op, cause: cause})
			var opts []redisstore.Option
			if op == "eval_ro" {
				opts = append(opts, redisstore.WithStrictExactReads())
			}
			store, err := redisstore.New(client, opts...)
			require.NoError(t, err)
			ctx := t.Context()
			switch op {
			case "exists", "eval_ro":
				_, err = store.IsRevoked(ctx, key)
			case "set":
				err = store.Revoke(ctx, key)
			case "eval":
				err = store.RevokeUntil(ctx, key, time.Second)
			case "del":
				err = store.Restore(ctx, key)
			default:
				for _, scanErr := range store.RawSource().StreamValues(ctx) {
					err = scanErr
				}
			}
			require.ErrorIs(t, err, cause)
			if op == "role" {
				require.ErrorIs(t, err, redisstore.ErrUnsupportedClient)
			}
			require.NotContains(t, err.Error(), key)
			require.NotContains(t, err.Error(), secret)
		})
	}
}
