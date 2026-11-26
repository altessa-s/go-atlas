// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/redistest"
	"github.com/altessa-s/go-atlas/reliability/saga"

	sagaredis "github.com/altessa-s/go-atlas/reliability/saga/storages/redis"
	goredis "github.com/redis/go-redis/v9"
)

// slotOf computes the Redis Cluster hash slot of key: CRC16 (XMODEM) of the
// first non-empty {hash tag}, or of the whole key, modulo 16384.
func slotOf(key string) int {
	if open := strings.IndexByte(key, '{'); open >= 0 {
		if end := strings.IndexByte(key[open+1:], '}'); end > 0 {
			key = key[open+1 : open+1+end]
		}
	}
	var crc uint16
	for i := range len(key) {
		crc ^= uint16(key[i]) << 8
		for range 8 {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return int(crc % 16384)
}

func TestSlotOf(t *testing.T) {
	t.Parallel()
	// Reference values from the Redis Cluster specification and CLUSTER KEYSLOT.
	require.Equal(t, 12182, slotOf("foo"))
	require.Equal(t, 12182, slotOf("{foo}bar"))
	require.Equal(t, slotOf("{user1000}.following"), slotOf("{user1000}.followers"))
}

// scriptKeys records the keys of every script call and the keys of every
// single-key command, as the cluster would route them.
type scriptKeys struct {
	mu      sync.Mutex
	scripts [][]string
	all     []string
}

func (h *scriptKeys) DialHook(next goredis.DialHook) goredis.DialHook { return next }

func (h *scriptKeys) record(cmd goredis.Cmder) {
	args := cmd.Args()
	name := strings.ToLower(cmd.Name())
	h.mu.Lock()
	defer h.mu.Unlock()
	switch name {
	case "eval", "evalsha":
		n, err := strconv.Atoi(fmt.Sprint(args[2]))
		if err != nil {
			return
		}
		keys := make([]string, 0, n)
		for _, a := range args[3 : 3+n] {
			keys = append(keys, fmt.Sprint(a))
		}
		h.scripts = append(h.scripts, keys)
		h.all = append(h.all, keys...)
	case "get", "set", "hget", "hmget", "exists", "pexpire", "sadd":
		if k, ok := args[1].(string); ok {
			h.all = append(h.all, k)
		}
	}
}

func (h *scriptKeys) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return func(ctx context.Context, cmd goredis.Cmder) error {
		h.record(cmd)
		return next(ctx, cmd)
	}
}

func (h *scriptKeys) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []goredis.Cmder) error {
		for _, c := range cmds {
			h.record(c)
		}
		return next(ctx, cmds)
	}
}

// Every script touches keys of one slot, the keys of one definition share it,
// and the default (untagged) prefix needs no manual hash tag.
func TestScriptsTouchOneSlotPerDefinition(t *testing.T) {
	t.Parallel()
	client, _ := redistest.RedisClient(t)
	hook := &scriptKeys{}
	client.AddHook(hook)
	s, err := sagaredis.New(client)
	require.NoError(t, err)
	ctx := t.Context()

	slots := map[string]int{}
	for _, def := range []string{"place-order", "ship-order", "pay{ment}", ""} {
		inst := instance("id-"+def, saga.StatusCompensating, time.Time{})
		inst.Definition = def
		require.NoError(t, s.Create(ctx, inst))
		got, err := s.Get(ctx, inst.ID)
		require.NoError(t, err)
		require.NoError(t, s.Update(ctx, got))
		_, err = s.FetchRecoverable(ctx, def, baseTime, 10)
		require.NoError(t, err)
		got.Status = saga.StatusCompleted
		require.NoError(t, s.Update(ctx, got))
		n, err := s.DeleteTerminal(ctx, def, time.Now().Add(time.Hour), 10)
		require.NoError(t, err)
		require.Equal(t, 1, n)
	}

	hook.mu.Lock()
	defer hook.mu.Unlock()
	require.NotEmpty(t, hook.scripts)
	for _, keys := range hook.scripts {
		require.NotEmpty(t, keys)
		for _, k := range keys {
			require.Equal(t, slotOf(keys[0]), slotOf(k), "script keys %v span slots", keys)
		}
		start := strings.Index(keys[0], "{d:")
		if start < 0 {
			continue // The ID lookup scripts address one plain key.
		}
		def := keys[0][start:]
		def = def[:strings.Index(def, "}")+1]
		if prev, ok := slots[def]; ok {
			require.Equal(t, prev, slotOf(keys[0]), "keys of %s must share one slot", def)
		}
		slots[def] = slotOf(keys[0])
	}
	require.Equal(t, slotOf("saga:{d:place-order}:i:x"), slotOf("saga:{d:place-order}:index:recoverable"))
	require.Equal(t, slotOf("saga:{d:place-order}:i:x"), slotOf("saga:{d:place-order}:index:terminal"))
	require.NotEqual(t, slotOf("saga:{d:place-order}:index:terminal"), slotOf("saga:{d:ship-order}:index:terminal"),
		"definitions must spread across slots")
	require.Len(t, slots, 4)
}

func TestClusterClientNeedsNoTaggedPrefix(t *testing.T) {
	t.Parallel()
	cluster := goredis.NewClusterClient(&goredis.ClusterOptions{Addrs: []string{"127.0.0.1:1"}})
	t.Cleanup(func() { _ = cluster.Close() })

	_, err := sagaredis.New(cluster)
	require.NoError(t, err)
	_, err = sagaredis.New(cluster, sagaredis.WithKeyPrefix("orders:"))
	require.NoError(t, err)
}

func TestIDsAreUniqueAcrossDefinitions(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := t.Context()

	a := instance("same", saga.StatusRunning, time.Time{})
	require.NoError(t, s.Create(ctx, a))
	b := instance("same", saga.StatusRunning, time.Time{})
	b.Definition = "ship-order"
	require.ErrorIs(t, s.Create(ctx, b), saga.ErrInstanceExists)

	got, err := s.Get(ctx, "same")
	require.NoError(t, err)
	require.Equal(t, "place-order", got.Definition)

	require.NoError(t, s.Delete(ctx, "same"))
	require.NoError(t, s.Create(ctx, b), "the ID is free after Delete, for any definition")
	got, err = s.Get(ctx, "same")
	require.NoError(t, err)
	require.Equal(t, "ship-order", got.Definition)
}

func TestStaleLookupDoesNotBlockID(t *testing.T) {
	t.Parallel()
	client, mr := redistest.RedisClient(t)
	s, err := sagaredis.New(client)
	require.NoError(t, err)
	ctx := t.Context()

	// A create that died after claiming the ID, and a delete that died before
	// removing the lookup, both leave a lookup without a hash.
	require.NoError(t, mr.Set("saga:id:orphan", "place-order"))
	_, err = s.Get(ctx, "orphan")
	require.ErrorIs(t, err, saga.ErrInstanceNotFound)

	other := instance("orphan", saga.StatusRunning, time.Time{})
	other.Definition = "ship-order"
	require.NoError(t, s.Create(ctx, other))
	got, err := s.Get(ctx, "orphan")
	require.NoError(t, err)
	require.Equal(t, "ship-order", got.Definition)
}

func TestSpansAllDefinitionsWhenNoneIsGiven(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := t.Context()

	for _, def := range []string{"place-order", "ship-order"} {
		for _, id := range []string{"r", "t"} {
			inst := instance(def+"-"+id, saga.StatusCompensating, time.Time{})
			inst.Definition = def
			if id == "t" {
				inst.Status = saga.StatusCompleted
			}
			require.NoError(t, s.Create(ctx, inst))
		}
	}

	rec, err := s.FetchRecoverable(ctx, "", baseTime, 0)
	require.NoError(t, err)
	require.Equal(t, []string{"place-order-r", "ship-order-r"}, ids(rec))
	rec, err = s.FetchRecoverable(ctx, "", baseTime, 1)
	require.NoError(t, err)
	require.Len(t, rec, 1)

	n, err := s.DeleteTerminal(ctx, "", time.Now(), 1)
	require.NoError(t, err)
	require.Equal(t, 1, n, "the limit spans definitions")
	n, err = s.DeleteTerminal(ctx, "", time.Now(), 0)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	_, err = s.Get(ctx, "place-order-t")
	require.ErrorIs(t, err, saga.ErrInstanceNotFound)
	_, err = s.Get(ctx, "ship-order-t")
	require.ErrorIs(t, err, saga.ErrInstanceNotFound)
}

func TestTerminalInstanceExpiresWithItsLookup(t *testing.T) {
	t.Parallel()
	client, mr := redistest.RedisClient(t)
	s, err := sagaredis.New(client, sagaredis.WithTTL(time.Second))
	require.NoError(t, err)
	ctx := t.Context()

	require.NoError(t, s.Create(ctx, instance("done", saga.StatusCompleted, time.Time{})))
	require.True(t, mr.Exists("saga:id:done"))
	mr.FastForward(2 * time.Second)
	require.False(t, mr.Exists("saga:id:done"), "the lookup must not outlive the instance it names")
}
