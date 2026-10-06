// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisfilter

import (
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/alicebob/miniredis/v2/server"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/probfilter"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	goredis "github.com/redis/go-redis/v9"
)

// newLeaseCores returns two Cores ("processes") of the same live key "t:f" on
// one miniredis, with T.RESERVE faked and the live key holding "old".
func newLeaseCores(t *testing.T) (*miniredis.Miniredis, *Core, *Core) {
	t.Helper()
	mr := miniredis.RunT(t)
	require.NoError(t, mr.Server().Register("T.RESERVE", func(c *server.Peer, _ string, _ []string) { c.WriteOK() }))
	require.NoError(t, mr.Set("t:f", "old"))
	newCore := func() *Core {
		client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
		t.Cleanup(func() { _ = client.Close() })
		return New(client, "t:f", Commands{Label: "Test", Reserve: "T.RESERVE"})
	}
	return mr, newCore(), newCore()
}

// stageUnderLease begins a rebuild on c and stages a replacement whose
// contents are the string value.
func stageUnderLease(t *testing.T, mr *miniredis.Miniredis, c *Core, value string) (*Lease, *Staging) {
	t.Helper()
	lease, err := c.BeginRebuild(t.Context())
	require.NoError(t, err)
	st, err := lease.Stage(t.Context())
	require.NoError(t, err)
	require.NoError(t, mr.Set(st.Key(), value))
	return lease, st
}

func TestLease_SecondRebuildIsRefusedWhileHeld(t *testing.T) {
	t.Parallel()
	mr, a, b := newLeaseCores(t)

	leaseA, err := a.BeginRebuild(t.Context())
	require.NoError(t, err)
	_, err = b.BeginRebuild(t.Context())
	require.ErrorIs(t, err, probfilter.ErrRebuildInProgress)

	require.NoError(t, leaseA.Release(t.Context()))
	require.False(t, mr.Exists(a.leaseKey), "Release frees the lease")
	leaseB, err := b.BeginRebuild(t.Context())
	require.NoError(t, err)
	require.Greater(t, leaseB.ticket, leaseA.ticket, "tickets grow")
	require.NoError(t, leaseB.Release(t.Context()))
}

// TestLease_ExpiredHolderCannotPublish reproduces OIDC-016: A loads an older
// snapshot and stalls until its lease expires; B takes the lease, publishes a
// newer snapshot; A resumes and tries to commit. A must be rejected and B's
// contents must stay live.
func TestLease_ExpiredHolderCannotPublish(t *testing.T) {
	t.Parallel()
	mr, a, b := newLeaseCores(t)

	leaseA, stA := stageUnderLease(t, mr, a, "from-A-older")
	mr.FastForward(LeaseTTL + time.Second) // A stalled; renewals stopped (frozen process)
	leaseA.stopOnce.Do(func() { close(leaseA.stop) })

	leaseB, stB := stageUnderLease(t, mr, b, "from-B-newer")
	require.NoError(t, stB.Commit(t.Context()))
	require.NoError(t, leaseB.Release(t.Context()))

	require.ErrorIs(t, stA.Commit(t.Context()), probfilter.ErrRebuildSuperseded)
	got, err := mr.Get("t:f")
	require.NoError(t, err)
	require.Equal(t, "from-B-newer", got, "an expired lease holder must not overwrite a newer snapshot")
	require.True(t, mr.Exists(stA.Key()), "a superseded commit renames nothing (the coordinator then aborts it)")
	require.NoError(t, stA.Abort(t.Context()))
	require.NoError(t, leaseA.Release(t.Context()))
}

// TestLease_PartialNewerCommitFencesOlderTicket fails B's commit right after
// the rename (before the marker), then lets A, with an older ticket, hold the
// lease again: the committed-ticket fence written before the rename must
// still reject A.
func TestLease_PartialNewerCommitFencesOlderTicket(t *testing.T) {
	t.Parallel()
	mr, a, b := newLeaseCores(t)

	leaseA, err := a.BeginRebuild(t.Context())
	require.NoError(t, err)
	stA, err := a.Stage(t.Context())
	require.NoError(t, err)
	stA.ticket = leaseA.ticket
	require.NoError(t, mr.Set(stA.Key(), "from-A-older"))
	require.NoError(t, leaseA.Release(t.Context()))

	leaseB, stB := stageUnderLease(t, mr, b, "from-B-newer")
	src := strings.Replace(commitScriptSource, "redis.call('SET', KEYS[3]", "do return redis.error_reply('ERR injected') end --", 1)
	require.NotEqual(t, commitScriptSource, src)
	stB.commit = goredis.NewScript(src)
	require.ErrorIs(t, stB.Commit(t.Context()), probfilter.ErrCommitIndeterminate)
	got, err := mr.Get("t:f")
	require.NoError(t, err)
	require.Equal(t, "from-B-newer", got)
	require.NoError(t, leaseB.Release(t.Context()))

	// A's ticket holds the lease again (e.g. a delayed request); the fence
	// written before B's rename still rejects it.
	require.NoError(t, mr.Set(a.leaseKey, "1"))
	require.Equal(t, int64(1), leaseA.ticket)
	require.ErrorIs(t, stA.Commit(t.Context()), probfilter.ErrRebuildSuperseded)
	got, err = mr.Get("t:f")
	require.NoError(t, err)
	require.Equal(t, "from-B-newer", got)
}

func TestLease_SameTicketRetryResumes(t *testing.T) {
	t.Parallel()
	mr, a, _ := newLeaseCores(t)
	lease, st := stageUnderLease(t, mr, a, "new")

	// A first attempt raises the fence and fails before the rename.
	src := strings.Replace(commitScriptSource, "redis.call('PERSIST'", "do return redis.error_reply('ERR injected') end --", 1)
	_, err := goredis.NewScript(src).Run(t.Context(), st.core.client, commitKeys(st), int64(60000), "1", "").Result()
	require.ErrorContains(t, err, "injected")

	require.NoError(t, st.Commit(t.Context()), "the same rebuild may resume its commit")
	got, err := mr.Get("t:f")
	require.NoError(t, err)
	require.Equal(t, "new", got)
	require.NoError(t, lease.Release(t.Context()))
}

func TestLease_RenewalKeepsLeaseAlive(t *testing.T) {
	t.Parallel()
	mr, a, b := newLeaseCores(t)
	a.leaseTTL = 300 * time.Millisecond

	lease, err := a.BeginRebuild(t.Context())
	require.NoError(t, err)
	mr.FastForward(250 * time.Millisecond)
	// The renewer ticks every 100ms and restores the full TTL.
	testhelpers.WaitFor(t, 5*time.Second, func() bool { return mr.TTL(a.leaseKey) > 50*time.Millisecond },
		"the renewer never extended the lease")
	mr.FastForward(250 * time.Millisecond)
	require.True(t, mr.Exists(a.leaseKey), "a renewed lease outlives its first TTL")

	_, err = b.BeginRebuild(t.Context())
	require.ErrorIs(t, err, probfilter.ErrRebuildInProgress)
	require.NoError(t, lease.Release(t.Context()))
}

func TestLease_KeysHaveNoUnexpectedTTL(t *testing.T) {
	t.Parallel()
	mr, a, _ := newLeaseCores(t)
	lease, st := stageUnderLease(t, mr, a, "new")
	require.NoError(t, st.Commit(t.Context()))
	require.NoError(t, lease.Release(t.Context()))
	require.Zero(t, mr.TTL(a.seqKey))
	require.Zero(t, mr.TTL(a.committedKey))
	require.Zero(t, mr.TTL("t:f"))
}

// TestLease_ExpiredHolderCannotPublishWhileSuccessorLoads lets A's lease
// expire while B holds the lease but has not committed yet: A's commit must
// already be rejected (only the current holder may publish), so it cannot
// publish its older snapshot before B's newer one.
func TestLease_ExpiredHolderCannotPublishWhileSuccessorLoads(t *testing.T) {
	t.Parallel()
	mr, a, b := newLeaseCores(t)

	leaseA, stA := stageUnderLease(t, mr, a, "from-A-older")
	leaseA.stopOnce.Do(func() { close(leaseA.stop) }) // A froze
	mr.FastForward(LeaseTTL + time.Second)

	leaseB, err := b.BeginRebuild(t.Context()) // B starts loading
	require.NoError(t, err)

	require.ErrorIs(t, stA.Commit(t.Context()), probfilter.ErrRebuildSuperseded)
	got, err := mr.Get("t:f")
	require.NoError(t, err)
	require.Equal(t, "old", got)
	require.NoError(t, leaseB.Release(t.Context()))
	require.NoError(t, leaseA.Release(t.Context()))
}
