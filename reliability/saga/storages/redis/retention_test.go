// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis_test

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/redistest"
	"github.com/altessa-s/go-atlas/reliability/saga"

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

// observeHook records the element count of every successful reply, for both the
// generic script form and the string-slice form, so a regression to a different
// command cannot hide from the count.
type observeHook struct {
	observed atomic.Int64
	elements atomic.Int64
	onReply  func(cmd goredis.Cmder)
}

func (h *observeHook) DialHook(next goredis.DialHook) goredis.DialHook { return next }

func (h *observeHook) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return func(ctx context.Context, cmd goredis.Cmder) error {
		err := next(ctx, cmd)
		if err != nil {
			return err
		}
		switch c := cmd.(type) {
		case *goredis.Cmd:
			if vals, ok := c.Val().([]any); ok {
				h.observed.Add(1)
				h.elements.Add(int64(len(vals)))
			}
		case *goredis.StringSliceCmd:
			h.observed.Add(1)
			h.elements.Add(int64(len(c.Val())))
		}
		if h.onReply != nil {
			h.onReply(cmd)
		}
		return nil
	}
}

func (h *observeHook) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
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

	hook := &observeHook{}
	client.AddHook(hook)
	rec, err := s.FetchRecoverable(ctx, "place-order", baseTime, 1)
	require.NoError(t, err)
	require.Len(t, rec, 1)
	require.Positive(t, hook.observed.Load(), "the scan must have been observed")
	// A page is member and score pairs; the scan must stop after the first page.
	require.LessOrEqual(t, hook.elements.Load(), int64(2*64))
}

func TestFetchRecoverableReportsCorruptRecordOnce(t *testing.T) {
	t.Parallel()
	client, _ := redistest.RedisClient(t)
	s, err := sagaredis.New(client)
	require.NoError(t, err)
	ctx := t.Context()

	require.NoError(t, s.Create(ctx, instance("a-corrupt", saga.StatusCompensating, time.Time{})))
	require.NoError(t, s.Create(ctx, instance("b-valid", saga.StatusCompensating, time.Time{})))
	require.NoError(t, client.HSet(ctx, "saga:{d:place-order}:i:a-corrupt", "d", "{not json").Err())

	rec, err := s.FetchRecoverable(ctx, "place-order", baseTime, 10)
	require.NoError(t, err)
	require.Equal(t, []string{"a-corrupt", "b-valid"}, ids(rec))
	stub := rec[0]
	require.True(t, stub.Corrupt)

	// The quarantine write is version-checked and removes it from the scan.
	stale := &saga.Instance{ID: stub.ID, Version: stub.Version - 1, Status: saga.StatusFailed}
	require.ErrorIs(t, s.Update(ctx, stale), saga.ErrVersionConflict)
	require.NoError(t, s.Update(ctx, &saga.Instance{ID: stub.ID, Version: stub.Version, Status: saga.StatusFailed, Definition: "place-order"}))
	rec, err = s.FetchRecoverable(ctx, "place-order", baseTime, 10)
	require.NoError(t, err)
	require.Equal(t, []string{"b-valid"}, ids(rec))
}

func TestFetchRecoverableContinuesWhenCursorMemberDisappears(t *testing.T) {
	t.Parallel()
	client, mr := redistest.RedisClient(t)
	s, err := sagaredis.New(client)
	require.NoError(t, err)
	ctx := t.Context()

	// Two full pages of equal-score entries, then a short third one.
	for i := range 130 {
		require.NoError(t, s.Create(ctx, instance(fmt.Sprintf("a-%03d", i), saga.StatusCompensating, time.Time{})))
	}

	var pages atomic.Int32
	removed := atomic.Bool{}
	client.AddHook(&observeHook{onReply: func(cmd goredis.Cmder) {
		if cmd.Name() != "evalsha" && cmd.Name() != "eval" {
			return
		}
		if pages.Add(1) == 2 && removed.CompareAndSwap(false, true) {
			for _, key := range mr.Keys() {
				if strings.HasSuffix(key, "index:recoverable") {
					_, _ = mr.ZRem(key, "a-127") // The cursor member: last of the second page.
				}
			}
		}
	}})

	rec, err := s.FetchRecoverable(ctx, "place-order", baseTime, 0)
	require.NoError(t, err)
	require.True(t, removed.Load(), "the cursor member must have been removed mid-scan")
	require.Len(t, rec, 130, "the scan continues past the removed cursor member")
	require.Equal(t, "a-129", rec[len(rec)-1].ID)
}

func TestTerminalIndexIsTrimmedWhenTTLExpiresHashes(t *testing.T) {
	t.Parallel()
	client, mr := redistest.RedisClient(t)
	s, err := sagaredis.New(client, sagaredis.WithTTL(time.Second))
	require.NoError(t, err)
	ctx := t.Context()

	old := instance("old", saga.StatusCompleted, time.Time{}) // Updated in 2023: its hash expired long ago.
	require.NoError(t, s.Create(ctx, old))
	mr.FastForward(2 * time.Second)

	fresh := instance("fresh", saga.StatusCompleted, time.Time{})
	fresh.UpdatedAt = time.Now()
	require.NoError(t, s.Create(ctx, fresh))

	members, err := mr.ZMembers("saga:{d:place-order}:index:terminal")
	require.NoError(t, err)
	require.Equal(t, []string{"fresh"}, members, "an expired instance must not leave a permanent index entry")
}

func TestExpiredLeaseIsRecoverableWithinTheSameSecond(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := t.Context()

	released := instance("released", saga.StatusRunning, time.Time{})
	released.LeaseOwner = "owner"
	released.LeaseUntil = baseTime.Add(500 * time.Millisecond)
	require.NoError(t, s.Create(ctx, released))

	rec, err := s.FetchRecoverable(ctx, "", baseTime.Add(400*time.Millisecond), 0)
	require.NoError(t, err)
	require.Empty(t, rec, "an active lease excludes the instance")
	rec, err = s.FetchRecoverable(ctx, "", baseTime.Add(501*time.Millisecond), 0)
	require.NoError(t, err)
	require.Equal(t, []string{"released"}, ids(rec), "a lease released within this second is due at once")
}

func TestDeleteTerminalKeepsReplacedRecord(t *testing.T) {
	t.Parallel()
	client, _ := redistest.RedisClient(t)
	s, err := sagaredis.New(client)
	require.NoError(t, err)
	ctx := t.Context()
	cutoff := baseTime.Add(time.Hour)

	for _, tc := range []struct {
		name    string
		replace func() *saga.Instance
	}{
		{"live replacement", func() *saga.Instance { return instance("a", saga.StatusRunning, time.Time{}) }},
		{"newer terminal replacement", func() *saga.Instance {
			inst := instance("a", saga.StatusCompleted, time.Time{})
			inst.UpdatedAt = cutoff.Add(time.Hour)
			return inst
		}},
		{"other definition replacement", func() *saga.Instance {
			inst := instance("a", saga.StatusCompleted, time.Time{})
			inst.Definition = "ship-order"
			return inst
		}},
	} {
		require.NoError(t, s.Create(ctx, instance("a", saga.StatusCompleted, time.Time{})))
		var once atomic.Bool
		hook := &pipelineHook{after: func() {
			if once.CompareAndSwap(false, true) { // Between the load and the delete.
				require.NoError(t, s.Delete(ctx, "a"))
				require.NoError(t, s.Create(ctx, tc.replace()))
			}
		}}
		client.AddHook(hook)

		n, err := s.DeleteTerminal(ctx, "place-order", cutoff, 0)
		require.NoError(t, err, tc.name)
		require.Zero(t, n, tc.name)
		_, err = s.Get(ctx, "a")
		require.NoError(t, err, tc.name+": the replacement must survive")
		require.True(t, once.Load(), tc.name)
		hook.disabled.Store(true)
		require.NoError(t, s.Delete(ctx, "a"))
	}
}

// pipelineHook runs a function after each pipeline completes.
type pipelineHook struct {
	after    func()
	disabled atomic.Bool
}

func (h *pipelineHook) DialHook(next goredis.DialHook) goredis.DialHook          { return next }
func (h *pipelineHook) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook { return next }
func (h *pipelineHook) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []goredis.Cmder) error {
		err := next(ctx, cmds)
		if !h.disabled.Load() {
			h.after()
		}
		return err
	}
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

// The index is a coarse filter at millisecond precision; Instance.Recoverable
// decides exactly. A lease expiring inside a millisecond must be returned by a
// query at that same instant or later in the same millisecond, never hidden
// until the next one.
func TestLeaseReleasedAtQueryInstantIsRecoverable(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := t.Context()

	released := instance("released", saga.StatusRunning, time.Time{})
	released.LeaseOwner = "owner"
	released.LeaseUntil = baseTime.Add(500*time.Millisecond + 300*time.Microsecond)
	require.NoError(t, s.Create(ctx, released))

	for _, now := range []time.Time{released.LeaseUntil, released.LeaseUntil.Add(time.Nanosecond), baseTime.Add(500*time.Millisecond + 900*time.Microsecond)} {
		rec, err := s.FetchRecoverable(ctx, "", now, 0)
		require.NoError(t, err)
		require.Equal(t, []string{"released"}, ids(rec), now.String())
	}
	rec, err := s.FetchRecoverable(ctx, "", released.LeaseUntil.Add(-time.Nanosecond), 0)
	require.NoError(t, err)
	require.Empty(t, rec, "a lease still active at the query instant excludes the instance")
}
