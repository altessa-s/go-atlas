// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisfilter_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/alicebob/miniredis/v2/server"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/probfilter"
	"github.com/altessa-s/go-atlas/data/probfilter/internal/redisfilter"

	goredis "github.com/redis/go-redis/v9"
)

var errLostReply = errors.New("i/o timeout")

// lostReplyHook lets script commands run on the server and then reports a
// network error, as if the reply was lost. When failChecks is set, follow-up
// EXISTS calls fail too.
type lostReplyHook struct {
	failChecks bool
}

func (lostReplyHook) DialHook(next goredis.DialHook) goredis.DialHook { return next }

func (h lostReplyHook) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return func(ctx context.Context, cmd goredis.Cmder) error {
		name := strings.ToLower(cmd.Name())
		if h.failChecks && name == "exists" {
			cmd.SetErr(errLostReply)
			return errLostReply
		}
		err := next(ctx, cmd)
		if (name == "evalsha" || name == "eval") && isCommit(cmd) {
			// Retry of EVALSHA as EVAL happens inside go-redis; report the
			// loss only once the script actually ran.
			if err == nil {
				cmd.SetErr(errLostReply)
				return errLostReply
			}
		}
		return err
	}
}

// isCommit recognizes the commit script call by its seven keys.
func isCommit(cmd goredis.Cmder) bool {
	args := cmd.Args()
	return len(args) > 2 && fmt.Sprint(args[2]) == "7"
}

func (lostReplyHook) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return next
}

// stagedForCommit returns a staging filter whose key holds a stand-in value,
// using a client with hook installed after staging.
func stagedForCommit(t *testing.T, hook goredis.Hook) (*miniredis.Miniredis, *redisfilter.Staging) {
	t.Helper()
	mr, client := newTestClient(t)
	fake := &fakeModule{}
	fake.register(t, mr, testCommands)
	require.NoError(t, mr.Set("t:f", "old"))

	st, err := redisfilter.New(client, "t:f", testCommands).Stage(t.Context(), int64(10))
	require.NoError(t, err)
	require.NoError(t, mr.Set(st.Key(), "new"))
	if hook != nil {
		client.AddHook(hook)
	}
	return mr, st
}

func TestStaging_Commit_LostReplyReconciledAsCommitted(t *testing.T) {
	t.Parallel()
	mr, st := stagedForCommit(t, lostReplyHook{})

	require.NoError(t, st.Commit(t.Context()), "the commit marker proves the rename happened")
	got, err := mr.Get("t:f")
	require.NoError(t, err)
	require.Equal(t, "new", got)
}

func TestStaging_Commit_LostReplyAndFailedCheckIsIndeterminate(t *testing.T) {
	t.Parallel()
	_, st := stagedForCommit(t, lostReplyHook{failChecks: true})

	err := st.Commit(t.Context())
	require.ErrorIs(t, err, probfilter.ErrCommitIndeterminate)
}

func TestStaging_Commit_MissingStagingIsIndeterminate(t *testing.T) {
	t.Parallel()
	mr, st := stagedForCommit(t, nil)
	mr.Del(st.Key())

	// Neither staging nor marker: a promotion whose reply and marker were
	// both lost looks the same, so non-promotion cannot be established.
	err := st.Commit(t.Context())
	require.ErrorIs(t, err, probfilter.ErrCommitIndeterminate)
	got, getErr := mr.Get("t:f")
	require.NoError(t, getErr)
	require.Equal(t, "old", got)
}

func TestCore_AddBatch_RejectsPerItemFailures(t *testing.T) {
	t.Parallel()

	replies := map[string]func(c *server.Peer, items int){
		"negative integer": func(c *server.Peer, items int) {
			c.WriteLen(items)
			for i := range items {
				if i == items-1 {
					c.WriteInt(-1)
					continue
				}
				c.WriteInt(1)
			}
		},
		"error element": func(c *server.Peer, items int) {
			c.WriteLen(items)
			for i := range items {
				if i == 0 {
					c.WriteError("ERR filter is full")
					continue
				}
				c.WriteInt(1)
			}
		},
	}

	for _, protocol := range []int{2, 3} {
		for name, reply := range replies {
			t.Run(fmt.Sprintf("RESP%d/%s", protocol, name), func(t *testing.T) {
				t.Parallel()
				mr := miniredis.RunT(t)
				fake := &fakeModule{created: true, batchReply: reply}
				fake.register(t, mr, testCommands)
				core := redisfilter.New(protocolClient(t, mr, protocol), "t:f", testCommands)

				err := core.AddBatch(t.Context(), slices.Values([]string{"a", "b", "c"}))
				require.ErrorIs(t, err, redisfilter.ErrItemRejected)
			})
		}
	}
}

func TestCore_AddBatch_AcceptsBooleanReplies(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	fake := &fakeModule{created: true, batchReply: func(c *server.Peer, items int) {
		c.WriteLen(items)
		for range items {
			writeBool(c, false) // RESP3: "already present"
		}
	}}
	fake.register(t, mr, testCommands)
	core := redisfilter.New(protocolClient(t, mr, 3), "t:f", testCommands)
	require.NoError(t, core.AddBatch(t.Context(), slices.Values([]string{"a", "b"})))
}

func TestCore_ReservesBeforeFirstWriteOnce(t *testing.T) {
	t.Parallel()
	mr, client := newTestClient(t)
	fake := &fakeModule{}
	fake.register(t, mr, testCommands)
	core := redisfilter.New(client, "t:f", testCommands, int64(500), "EXPANSION", int64(4))

	require.NoError(t, core.Add(t.Context(), "a"))
	require.NoError(t, core.AddBatch(t.Context(), slices.Values([]string{"b"})))
	require.NoError(t, core.Add(t.Context(), "c"))

	snap := fake.snapshot()
	require.Equal(t, [][]string{{"t:f", "500", "EXPANSION", "4"}}, snap.reserveCalls,
		"the live filter is reserved with the configured args once, before the first write")
}

func TestCore_ReserveFailureIsRetried(t *testing.T) {
	t.Parallel()
	mr, client := newTestClient(t)
	attempts := 0
	require.NoError(t, mr.Server().Register(testCommands.Reserve, func(c *server.Peer, _ string, _ []string) {
		attempts++
		if attempts == 1 {
			c.WriteError("ERR transient")
			return
		}
		c.WriteOK()
	}))
	require.NoError(t, mr.Server().Register(testCommands.Add, func(c *server.Peer, _ string, _ []string) { c.WriteInt(1) }))
	core := redisfilter.New(client, "t:f", testCommands)

	require.ErrorContains(t, core.Add(t.Context(), "a"), "create Redis Test filter")
	require.NoError(t, core.Add(t.Context(), "a"))
	require.NoError(t, core.Add(t.Context(), "b"))
	require.Equal(t, 2, attempts)
}
