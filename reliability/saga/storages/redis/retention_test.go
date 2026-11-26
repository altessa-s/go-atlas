// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/redistest"
	"github.com/altessa-s/go-atlas/reliability/saga"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	sagaredis "github.com/altessa-s/go-atlas/reliability/saga/storages/redis"
	goredis "github.com/redis/go-redis/v9"
)

func TestLiveInstanceSurvivesTTL(t *testing.T) {
	t.Parallel()
	client, mr := redistest.RedisClient(t)
	s, err := sagaredis.New(client, sagaredis.WithTTL(time.Second))
	require.NoError(t, err)
	ctx := t.Context()

	inst := instance("live", saga.StatusCompensating, time.Time{})
	require.NoError(t, s.Create(ctx, inst))
	inst.Stage = 0
	require.NoError(t, s.Update(ctx, inst))
	done := instance("done", saga.StatusCompleted, time.Time{})
	require.NoError(t, s.Create(ctx, done))

	mr.FastForward(2 * time.Second)

	_, err = s.Get(ctx, "live")
	require.NoError(t, err, "a non-terminal instance must not expire")
	_, err = s.Get(ctx, "done")
	require.ErrorIs(t, err, saga.ErrInstanceNotFound, "a terminal instance expires after the TTL")
}

// countingHook counts the elements of every script reply (page members).
type countingHook struct{ elements atomic.Int64 }

func (h *countingHook) DialHook(next goredis.DialHook) goredis.DialHook { return next }

func (h *countingHook) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return func(ctx context.Context, cmd goredis.Cmder) error {
		err := next(ctx, cmd)
		if c, ok := cmd.(*goredis.Cmd); ok {
			if vals, ok := c.Val().([]any); ok {
				h.elements.Add(int64(len(vals)))
			}
		}
		return err
	}
}

func (h *countingHook) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return next
}

func TestFetchRecoverableReadsBoundedPages(t *testing.T) {
	t.Parallel()
	client, _ := redistest.RedisClient(t)
	s, err := sagaredis.New(client)
	require.NoError(t, err)
	ctx := t.Context()

	const due = 400
	for i := range due {
		require.NoError(t, s.Create(ctx, instance(fmt.Sprintf("c-%03d", i), saga.StatusCompensating, time.Time{})))
	}

	hook := &countingHook{}
	client.AddHook(hook)
	rec, err := s.FetchRecoverable(ctx, "place-order", baseTime, 1)
	require.NoError(t, err)
	require.Len(t, rec, 1)
	// A page is member and score pairs; the scan must stop after the first page.
	require.LessOrEqual(t, hook.elements.Load(), int64(2*64))
}

func TestFetchRecoverableSkipsCorruptEntry(t *testing.T) {
	t.Parallel()
	client, _ := redistest.RedisClient(t)
	s, err := sagaredis.New(client)
	require.NoError(t, err)
	ctx := t.Context()

	require.NoError(t, s.Create(ctx, instance("a-corrupt", saga.StatusCompensating, time.Time{})))
	require.NoError(t, s.Create(ctx, instance("b-valid", saga.StatusCompensating, time.Time{})))
	require.NoError(t, client.HSet(ctx, "saga:i:a-corrupt", "d", "{not json").Err())

	rec, err := s.FetchRecoverable(ctx, "place-order", baseTime, 10)
	require.NoError(t, err)
	require.Equal(t, []string{"b-valid"}, ids(rec))
}

func TestInvalidUTF8IDCannotAddressTwin(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := t.Context()

	const twinID, invalidID = "x�y", "x\xffy"
	twin := instance(twinID, saga.StatusRunning, time.Time{})
	twin.LastError = "twin"
	require.NoError(t, s.Create(ctx, twin))

	bad := instance(invalidID, saga.StatusRunning, time.Time{})
	require.ErrorIs(t, s.Create(ctx, bad), saga.ErrInvalidStorageKey)
	require.ErrorIs(t, s.Update(ctx, bad), saga.ErrInvalidStorageKey)
	_, err := s.Get(ctx, invalidID)
	require.ErrorIs(t, err, saga.ErrInvalidStorageKey)
	require.ErrorIs(t, s.Delete(ctx, invalidID), sagaredis.ErrInvalidID)

	got, err := s.Get(ctx, twinID)
	require.NoError(t, err)
	require.Equal(t, "twin", got.LastError)
	require.Equal(t, twin.Version, got.Version)
}

func TestNewRejectsUntaggedClusterPrefix(t *testing.T) {
	t.Parallel()
	cluster := goredis.NewClusterClient(&goredis.ClusterOptions{Addrs: []string{"127.0.0.1:1"}})
	t.Cleanup(func() { _ = cluster.Close() })

	_, err := sagaredis.New(cluster)
	require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
	_, err = sagaredis.New(cluster, sagaredis.WithKeyPrefix("{saga"))
	require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
	_, err = sagaredis.New(cluster, sagaredis.WithKeyPrefix("{}saga:"))
	require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
	_, err = sagaredis.New(cluster, sagaredis.WithKeyPrefix("{saga}:"))
	require.NoError(t, err)

	client, _ := redistest.RedisClient(t)
	_, err = sagaredis.New(client)
	require.NoError(t, err, "a standalone client needs no hash tag")
}
