// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisfilter

import (
	"slices"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/alicebob/miniredis/v2/server"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/cache/probfilter"

	goredis "github.com/redis/go-redis/v9"
)

var cuckooLikeCommands = Commands{
	Label:        "Test",
	Add:          "T.INSERT",
	AddTokens:    []string{"NOCREATE", "ITEMS"},
	AddBatch:     "T.INSERT",
	BatchTokens:  []string{"NOCREATE", "ITEMS"},
	FalseRejects: true,
	JournalAdds:  true,
	Reserve:      "T.RESERVE",
}

// newCuckooLikeCore returns a Core whose fake T.INSERT answers -1, like a full
// Cuckoo filter, for every item that appears in rejected on the live key (or,
// when rejectStaging is set, on any other key).
func newCuckooLikeCore(t *testing.T, rejected []string, rejectStaging bool) (*miniredis.Miniredis, *Core) {
	t.Helper()
	mr := miniredis.RunT(t)
	require.NoError(t, mr.Server().Register("T.RESERVE", func(c *server.Peer, _ string, _ []string) { c.WriteOK() }))
	require.NoError(t, mr.Server().Register("T.INSERT", func(c *server.Peer, _ string, args []string) {
		key := args[0]
		items := args[len(args)-(len(args)-1-slices.Index(args, "ITEMS")):]
		c.WriteLen(len(items))
		for _, v := range items {
			full := slices.Contains(rejected, v) && ((key == "t:f") != rejectStaging)
			if full {
				c.WriteInt(-1)
				continue
			}
			c.WriteInt(1)
		}
	}))
	require.NoError(t, mr.Set("t:f", "old"))
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return mr, New(client, "t:f", cuckooLikeCommands)
}

// TestJournaledInsert_JournalsOnlyAcceptedItems pins that a mixed-success live
// batch journals the accepted values only.
func TestJournaledInsert_JournalsOnlyAcceptedItems(t *testing.T) {
	t.Parallel()
	mr, core := newCuckooLikeCore(t, []string{"full"}, false)

	lease, err := core.BeginRebuild(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = lease.Release(t.Context()) })

	err = core.AddBatch(t.Context(), slices.Values([]string{"a", "full", "b"}))
	require.ErrorIs(t, err, ErrItemRejected)

	journal, err := mr.List(core.journalKey)
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, journal)
	require.Positive(t, mr.TTL(core.journalKey), "the journal expires with its lease generation")
}

// TestStagingCommit_FullStagingFailsInsteadOfDroppingJournaledAdd pins that a
// journaled value the staging filter rejects fails the rebuild: neither the
// drain nor the commit may treat the rejection as a success.
func TestStagingCommit_FullStagingFailsInsteadOfDroppingJournaledAdd(t *testing.T) {
	t.Parallel()
	mr, core := newCuckooLikeCore(t, []string{"x"}, true)

	lease, err := core.BeginRebuild(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = lease.Release(t.Context()) })
	st, err := lease.Stage(t.Context())
	require.NoError(t, err)
	require.NoError(t, mr.Set(st.Key(), "new"))

	require.NoError(t, core.Add(t.Context(), "x"), "the live filter accepts it")

	err = st.Commit(t.Context())
	require.Error(t, err)
	got, getErr := mr.Get("t:f")
	require.NoError(t, getErr)
	require.Equal(t, "old", got, "a failed commit leaves the live filter")
	items, listErr := mr.List(core.journalKey)
	require.NoError(t, listErr)
	require.Equal(t, []string{"x"}, items, "the rejected value must not be trimmed from the journal")
}

// TestJournal_ExpiresWithoutSuccessor pins that a crashed rebuild leaves no
// journal behind once its lease and the grace period have passed, and that a
// live holder keeps it by renewing.
func TestJournal_ExpiresWithoutSuccessor(t *testing.T) {
	t.Parallel()
	mr, core := newCuckooLikeCore(t, nil, false)

	lease, err := core.BeginRebuild(t.Context())
	require.NoError(t, err)
	lease.stopOnce.Do(func() { close(lease.stop) }) // a crashed holder stops renewing
	<-lease.stopped
	require.NoError(t, core.Add(t.Context(), "a"))
	require.True(t, mr.Exists(core.journalKey))
	require.True(t, mr.Exists(core.journalGenKey))

	mr.FastForward(LeaseTTL + journalGrace + time.Second)
	require.False(t, mr.Exists(core.journalKey), "the abandoned journal expired")
	require.False(t, mr.Exists(core.journalGenKey))
}

// TestStagingCommit_LostJournalFailsClosed pins that a live rebuild whose
// journal generation vanished does not publish.
func TestStagingCommit_LostJournalFailsClosed(t *testing.T) {
	t.Parallel()
	mr, core := newCuckooLikeCore(t, nil, false)

	lease, err := core.BeginRebuild(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = lease.Release(t.Context()) })
	st, err := lease.Stage(t.Context())
	require.NoError(t, err)
	require.NoError(t, mr.Set(st.Key(), "new"))
	require.NoError(t, core.Add(t.Context(), "a"))

	mr.Del(core.journalKey)
	mr.Del(core.journalGenKey)

	err = st.Commit(t.Context())
	require.ErrorIs(t, err, probfilter.ErrRebuildSuperseded)
	require.ErrorIs(t, err, errJournalLost)
	got, getErr := mr.Get("t:f")
	require.NoError(t, getErr)
	require.Equal(t, "old", got)
}
