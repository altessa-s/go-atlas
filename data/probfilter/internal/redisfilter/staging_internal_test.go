// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisfilter

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/alicebob/miniredis/v2/server"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/probfilter"

	goredis "github.com/redis/go-redis/v9"
)

// White-box: slot computation is private.

func TestCRC16_RedisVectors(t *testing.T) {
	t.Parallel()
	// Reference values from the Redis Cluster specification.
	require.Equal(t, uint16(0x31C3), crc16("123456789"))
	require.Equal(t, uint16(12182), keySlot("foo"))
	require.Equal(t, keySlot("user1000"), keySlot("{user1000}.following"))
}

func TestStagingKey_SameSlot(t *testing.T) {
	t.Parallel()

	for _, live := range []string{
		"bloom:users",        // no tag: wrapped
		"t:{tenant}:f",       // valid tag: kept
		"t:{}:f",             // empty braces: whole key hashed, contains "}"
		"a}b",                // unmatched closing brace
		"a{b",                // unmatched opening brace
		"x{}{y}",             // first tag empty: Redis does not look further
		"}{",                 // closing before opening
		"cuckoo:{}:sessions", // prefix with empty braces
	} {
		t.Run(live, func(t *testing.T) {
			t.Parallel()
			for _, kind := range []string{"staging", "committed", "generation", "delete"} {
				key := metaKey(live, kind, "0123abcd")
				require.Equal(t, keySlot(live), keySlot(key), "%s key %q", kind, key)
				require.True(t, strings.HasPrefix(key, MetaPrefix))
			}
		})
	}
}

func TestCheckBatchReply(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		reply        any
		falseRejects bool
		wantErr      bool
	}{
		{name: "RESP2 ints", reply: []any{int64(1), int64(0)}},
		{name: "RESP3 bools", reply: []any{true, false}},
		{name: "full cuckoo item", reply: []any{int64(1), int64(-1)}, wantErr: true},
		{name: "full cuckoo item RESP3", reply: []any{true, false}, falseRejects: true, wantErr: true},
		{name: "cuckoo successes RESP3", reply: []any{true, true}, falseRejects: true},
		{name: "error element", reply: []any{int64(1), errTest("ERR filter is full")}, wantErr: true},
		{name: "non-array", reply: "OK"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := checkBatchReply(tc.reply, tc.falseRejects)
			if tc.wantErr {
				require.ErrorIs(t, err, ErrItemRejected)
				return
			}
			require.NoError(t, err)
		})
	}
}

type errTest string

func (e errTest) Error() string { return string(e) }

// stagedInternal returns a staging filter of key "t:f" whose staging key holds
// a stand-in value with a TTL; T.RESERVE is faked.
func stagedInternal(t *testing.T) (*miniredis.Miniredis, *Staging) {
	t.Helper()
	mr := miniredis.RunT(t)
	require.NoError(t, mr.Server().Register("T.RESERVE", func(c *server.Peer, _ string, _ []string) { c.WriteOK() }))
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	core := New(client, "t:f", Commands{Label: "Test", Reserve: "T.RESERVE"})
	require.NoError(t, mr.Set("t:f", "old"))
	st, err := core.Stage(t.Context())
	require.NoError(t, err)
	require.NoError(t, mr.Set(st.Key(), "new"))
	mr.SetTTL(st.Key(), StagingTTL)
	return mr, st
}

func TestStaging_Commit_ScriptFailsAfterRename(t *testing.T) {
	t.Parallel()
	mr, st := stagedInternal(t)
	st.commit = goredis.NewScript(`
redis.call('PERSIST', KEYS[1])
redis.call('RENAME', KEYS[1], KEYS[2])
return redis.error_reply('ERR injected after rename')
`)

	// Without the marker there is no positive evidence of promotion.
	require.ErrorIs(t, st.Commit(t.Context()), probfilter.ErrCommitIndeterminate)
	got, err := mr.Get("t:f")
	require.NoError(t, err)
	require.Equal(t, "new", got)
	require.Zero(t, mr.TTL("t:f"), "a promoted live key must not keep the staging TTL")
}

func TestStaging_Commit_ErrorWithExpiredStagingIsIndeterminate(t *testing.T) {
	t.Parallel()
	mr, st := stagedInternal(t)
	mr.Del(st.Key()) // expired before the commit
	st.commit = goredis.NewScript(`return redis.error_reply('NOPERM injected')`)

	require.ErrorIs(t, st.Commit(t.Context()), probfilter.ErrCommitIndeterminate,
		"a missing staging key alone is no evidence of promotion")
	got, err := mr.Get("t:f")
	require.NoError(t, err)
	require.Equal(t, "old", got)
}

func TestStaging_Commit_MarkerProvesPromotionAfterError(t *testing.T) {
	t.Parallel()
	mr, st := stagedInternal(t)
	st.commit = goredis.NewScript(`
redis.call('PERSIST', KEYS[1])
redis.call('RENAME', KEYS[1], KEYS[2])
redis.call('SET', KEYS[3], '1', 'PX', ARGV[1])
return redis.error_reply('ERR injected after marker')
`)

	require.NoError(t, st.Commit(t.Context()))
	got, err := mr.Get("t:f")
	require.NoError(t, err)
	require.Equal(t, "new", got)
}

func TestStaging_Commit_ScriptFailsBeforeRename(t *testing.T) {
	t.Parallel()
	mr, st := stagedInternal(t)
	st.commit = goredis.NewScript(`return redis.error_reply('ERR injected before rename')`)

	// A server error proves nothing about a delayed earlier attempt.
	require.ErrorIs(t, st.Commit(t.Context()), probfilter.ErrCommitIndeterminate)
	got, getErr := mr.Get("t:f")
	require.NoError(t, getErr)
	require.Equal(t, "old", got)
}

func TestStaging_Commit_IsIdempotent(t *testing.T) {
	t.Parallel()
	mr, st := stagedInternal(t)

	require.NoError(t, st.Commit(t.Context()))
	// A retry of the script after a lost reply finds the marker.
	require.NoError(t, st.Commit(t.Context()))
	got, err := mr.Get("t:f")
	require.NoError(t, err)
	require.Equal(t, "new", got)
	require.Zero(t, mr.TTL("t:f"))
}

// TestMetaKey_NoCollisionWithFilters checks that metadata keys of one filter
// can never equal another filter's key or another filter's metadata key, e.g.
// filters named "{tenant}:users" and "{tenant}:users:generation".
func TestMetaKey_NoCollisionWithFilters(t *testing.T) {
	t.Parallel()
	a, b := "cuckoo:{tenant}:users", "cuckoo:{tenant}:users:generation"
	require.NotEqual(t, b, metaKey(a, "generation", ""))
	require.NotEqual(t, metaKey(a, "generation", ""), metaKey(b, "generation", ""))
	require.NotEqual(t, metaKey(a, "staging", "1"), metaKey(b, "staging", "1"))
	require.Equal(t, keySlot(a), keySlot(metaKey(a, "generation", "")))
}

func TestNew_RejectsReservedNamespace(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	core := New(client, MetaPrefix+"x", Commands{Label: "Test"})

	_, err := core.MightExist(t.Context(), "v")
	require.ErrorIs(t, err, ErrReservedKey)
	require.ErrorIs(t, core.Add(t.Context(), "v"), ErrReservedKey)
	_, err = core.Delete(t.Context(), "T.DEL", "v")
	require.ErrorIs(t, err, ErrReservedKey)
	_, err = core.Stage(t.Context())
	require.ErrorIs(t, err, ErrReservedKey)
}

// TestDeleteScript_RetryOfSameRequestDeletesOnce replays the same delete
// request, as a client retry after a lost reply would: the delete command
// runs once and the retry returns the recorded outcome.
func TestDeleteScript_RetryOfSameRequestDeletesOnce(t *testing.T) {
	t.Parallel()
	_, client, core, dels := newDeleteCore(t)
	keys, reqID := deleteKeys(core), "req-1"

	for range 3 {
		reply, err := deleteScript.Run(t.Context(), client, keys, "", "T.DEL", "x", farDeadline, reqID, deletePruneBatch).Result()
		require.NoError(t, err)
		require.Equal(t, int64(1), reply)
	}
	require.Equal(t, int32(1), dels.Load(), "one delete request must run the delete command at most once")
}

func TestCommitScript_RefusesNonStringGenerationKey(t *testing.T) {
	t.Parallel()
	mr, _, core, _ := newDeleteCore(t)
	require.NoError(t, mr.Set("t:f", "old"))
	st, err := core.Stage(t.Context())
	require.NoError(t, err)
	require.NoError(t, mr.Set(st.Key(), "new"))
	_, err = mr.Push(core.genKey, "not-a-token")
	require.NoError(t, err)

	require.Error(t, st.Commit(t.Context()))
	got, err := mr.Get("t:f")
	require.NoError(t, err)
	require.Equal(t, "old", got)
}

// genSwapHook simulates a rebuild by another process landing between a
// delete's generation read and its script: right after the generation read
// it overwrites the generation token.
type genSwapHook struct {
	mr     *miniredis.Miniredis
	genKey string
}

func (genSwapHook) DialHook(next goredis.DialHook) goredis.DialHook { return next }

func (h genSwapHook) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return func(ctx context.Context, cmd goredis.Cmder) error {
		err := next(ctx, cmd)
		args := cmd.Args()
		name := strings.ToLower(cmd.Name())
		// The generation read: a script whose only key is the generation key.
		if err == nil && (name == "evalsha" || name == "eval") && len(args) > 3 &&
			fmt.Sprint(args[2]) == "1" && args[3] == h.genKey {
			_ = h.mr.Set(h.genKey, "rebuilt-meanwhile")
		}
		return err
	}
}

func (h genSwapHook) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []goredis.Cmder) error {
		err := next(ctx, cmds)
		for _, cmd := range cmds {
			if strings.EqualFold(cmd.Name(), "get") && len(cmd.Args()) > 1 && cmd.Args()[1] == h.genKey {
				_ = h.mr.Set(h.genKey, "rebuilt-meanwhile")
			}
		}
		return err
	}
}

func newDeleteCore(t *testing.T) (*miniredis.Miniredis, *goredis.Client, *Core, *atomic.Int32) {
	t.Helper()
	mr := miniredis.RunT(t)
	dels := &atomic.Int32{}
	require.NoError(t, mr.Server().Register("T.RESERVE", func(c *server.Peer, _ string, _ []string) { c.WriteOK() }))
	require.NoError(t, mr.Server().Register("T.DEL", func(c *server.Peer, _ string, _ []string) {
		dels.Add(1)
		c.WriteInt(1)
	}))
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return mr, client, New(client, "t:f", Commands{Label: "Test", Reserve: "T.RESERVE"}), dels
}

// TestCore_Delete_RebuildBetweenReadAndDeleteIsSkipped changes the generation
// between Core.Delete's token read and its script: the delete must not reach
// the replacement and must not be retried against it.
func TestCore_Delete_RebuildBetweenReadAndDeleteIsSkipped(t *testing.T) {
	t.Parallel()
	mr, client, core, dels := newDeleteCore(t)
	client.AddHook(genSwapHook{mr: mr, genKey: core.genKey})

	_, err := core.Delete(t.Context(), "T.DEL", "x")
	require.ErrorIs(t, err, ErrStaleGeneration)
	require.Zero(t, dels.Load(), "no delete may reach the replaced filter")
}

func TestCore_Delete_ReadsCurrentGeneration(t *testing.T) {
	t.Parallel()
	mr, _, core, dels := newDeleteCore(t)

	reply, err := core.Delete(t.Context(), "T.DEL", "x") // never rebuilt
	require.NoError(t, err)
	require.Equal(t, int64(1), reply)

	require.NoError(t, mr.Set(core.genKey, "other-process")) // another process rebuilt
	reply, err = core.Delete(t.Context(), "T.DEL", "x")
	require.NoError(t, err)
	require.Equal(t, int64(1), reply)
	require.Equal(t, int32(2), dels.Load())
}

// TestStaging_Commit_FailureAfterRenameStillFencesOldDeletes injects a failure
// right after the rename into the real commit script: the generation token
// must already have changed, so a delete carrying the old token is skipped.
func TestStaging_Commit_FailureAfterRenameStillFencesOldDeletes(t *testing.T) {
	t.Parallel()
	mr, client, core, dels := newDeleteCore(t)
	require.NoError(t, mr.Set("t:f", "old"))

	oldToken := "" // never rebuilt
	st, err := core.Stage(t.Context())
	require.NoError(t, err)
	require.NoError(t, mr.Set(st.Key(), "new"))

	src := strings.Replace(commitScriptSource, "redis.call('SET', KEYS[3]", "do return redis.error_reply('ERR injected') end --", 1)
	require.NotEqual(t, commitScriptSource, src)
	st.commit = goredis.NewScript(src)
	require.ErrorIs(t, st.Commit(t.Context()), probfilter.ErrCommitIndeterminate)
	got, err := mr.Get("t:f")
	require.NoError(t, err)
	require.Equal(t, "new", got, "the replacement was promoted before the failure")

	keys, reqID := deleteKeys(core), "late"
	reply, err := deleteScript.Run(t.Context(), client, keys, oldToken, "T.DEL", "x", farDeadline, reqID, deletePruneBatch).Result()
	require.NoError(t, err)
	require.Equal(t, int64(staleGeneration), reply)
	require.Zero(t, dels.Load(), "a delete aimed at the replaced filter must be skipped")
}

// TestStaging_Commit_GenerationWriteFailurePreventsPromotion injects a failure
// at the generation write of the real commit script: the replacement must not
// be promoted under the old token.
func TestStaging_Commit_GenerationWriteFailurePreventsPromotion(t *testing.T) {
	t.Parallel()
	mr, _, core, _ := newDeleteCore(t)
	require.NoError(t, mr.Set("t:f", "old"))
	st, err := core.Stage(t.Context())
	require.NoError(t, err)
	require.NoError(t, mr.Set(st.Key(), "new"))

	src := strings.Replace(commitScriptSource, "redis.call('INCR', KEYS[4])", "do return redis.error_reply('NOPERM injected') end --", 1)
	require.NotEqual(t, commitScriptSource, src)
	st.commit = goredis.NewScript(src)

	require.ErrorIs(t, st.Commit(t.Context()), probfilter.ErrCommitIndeterminate)
	got, err := mr.Get("t:f")
	require.NoError(t, err)
	require.Equal(t, "old", got, "no promotion without a new generation token")
}

// TestDeleteScript_OutcomeRecordFailureBlocksRetry injects a failure into the
// real delete script right after the delete command, before its outcome is
// recorded; a replay of the same request must not delete again.
func TestDeleteScript_OutcomeRecordFailureBlocksRetry(t *testing.T) {
	t.Parallel()
	_, client, core, dels := newDeleteCore(t)
	keys, reqID := deleteKeys(core), "req-2"

	src := strings.Replace(deleteScriptSource, "redis.call('HSET', KEYS[3], ARGV[5], tostring(r))",
		"do return redis.error_reply('NOPERM injected') end --", 1)
	require.NotEqual(t, deleteScriptSource, src)
	failing := goredis.NewScript(src)

	_, err := failing.Run(t.Context(), client, keys, "", "T.DEL", "x", farDeadline, reqID, deletePruneBatch).Result()
	require.ErrorContains(t, err, "NOPERM injected")
	require.Equal(t, int32(1), dels.Load())

	// The client retries the same request with the real script.
	_, err = deleteScript.Run(t.Context(), client, keys, "", "T.DEL", "x", farDeadline, reqID, deletePruneBatch).Result()
	require.ErrorContains(t, err, "PFPENDING")
	require.Equal(t, int32(1), dels.Load(), "a retry must not run the delete command again")
}

func TestDeleteScript_FailedDeleteClearsPending(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	calls := 0
	require.NoError(t, mr.Server().Register("T.DEL", func(c *server.Peer, _ string, _ []string) {
		calls++
		if calls == 1 {
			c.WriteError("ERR transient")
			return
		}
		c.WriteInt(1)
	}))
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	keys, reqID := deleteKeys(New(client, "t:f", Commands{Label: "Test"})), "req-3"

	_, err := deleteScript.Run(t.Context(), client, keys, "", "T.DEL", "x", farDeadline, reqID, deletePruneBatch).Result()
	require.ErrorContains(t, err, "transient")
	// Nothing was deleted, so the same request may run again.
	reply, err := deleteScript.Run(t.Context(), client, keys, "", "T.DEL", "x", farDeadline, reqID, deletePruneBatch).Result()
	require.NoError(t, err)
	require.Equal(t, int64(1), reply)
}

// farDeadline is a delete-request deadline far in the future.
const farDeadline = int64(1) << 50

// TestDeleteScript_LateRetryIsRejected replays a request after the server
// clock passed its deadline (and its record was pruned): it must not delete
// again.
func TestDeleteScript_LateRetryIsRejected(t *testing.T) {
	t.Parallel()
	mr, client, core, dels := newDeleteCore(t)
	keys, reqID := deleteKeys(core), "req-4"
	deadline := time.Now().Add(time.Minute).UnixMilli()

	reply, err := deleteScript.Run(t.Context(), client, keys, "", "T.DEL", "x", deadline, reqID, deletePruneBatch).Result()
	require.NoError(t, err)
	require.Equal(t, int64(1), reply)
	require.Equal(t, int32(1), dels.Load())

	mr.SetTime(time.Now().Add(2 * time.Minute))
	// Another request prunes the outdated record.
	_, err = deleteScript.Run(t.Context(), client, keys, "", "T.DEL", "y", farDeadline, "req-5", deletePruneBatch).Result()
	require.NoError(t, err)
	require.False(t, mr.Exists(core.deletesKey) && hashHas(t, mr, core.deletesKey, reqID), "outdated records are pruned")

	_, err = deleteScript.Run(t.Context(), client, keys, "", "T.DEL", "x", deadline, reqID, deletePruneBatch).Result()
	require.ErrorContains(t, err, "PFEXPIRED")
	require.Equal(t, int32(2), dels.Load(), "a late retry must not delete again")
}

// TestDeleteRecords_HaveNoTTL checks that delete records are stored without an
// expiry, so volatile-* eviction policies cannot drop them while their
// request can still be retried.
func TestDeleteRecords_HaveNoTTL(t *testing.T) {
	t.Parallel()
	mr, _, core, _ := newDeleteCore(t)
	_, err := core.Delete(t.Context(), "T.DEL", "x")
	require.NoError(t, err)
	require.True(t, mr.Exists(core.deletesKey))
	require.Zero(t, mr.TTL(core.deletesKey))
	require.Zero(t, mr.TTL(core.deadlinesKey))
}

func deleteKeys(core *Core) []string {
	return []string{core.filterKey, core.genKey, core.deletesKey, core.deadlinesKey}
}

func hashHas(t *testing.T, mr *miniredis.Miniredis, key, field string) bool {
	t.Helper()
	return mr.HGet(key, field) != ""
}

// TestDeleteScript_ReplayAtExactDeadlineIsRejected replays completed and
// pending requests at exactly their deadline millisecond, where pruning
// would remove their records: admission must reject them first.
func TestDeleteScript_ReplayAtExactDeadlineIsRejected(t *testing.T) {
	t.Parallel()

	for _, pending := range []bool{false, true} {
		t.Run(fmt.Sprintf("pending=%v", pending), func(t *testing.T) {
			t.Parallel()
			mr, client, core, dels := newDeleteCore(t)
			keys, reqID := deleteKeys(core), "req-6"
			start := time.Now().Truncate(time.Millisecond)
			mr.SetTime(start)
			deadline := start.Add(time.Minute).UnixMilli()

			script := deleteScript
			if pending {
				src := strings.Replace(deleteScriptSource, "redis.call('HSET', KEYS[3], ARGV[5], tostring(r))",
					"do return redis.error_reply('NOPERM injected') end --", 1)
				script = goredis.NewScript(src)
			}
			_, _ = script.Run(t.Context(), client, keys, "", "T.DEL", "x", deadline, reqID, deletePruneBatch).Result()
			require.Equal(t, int32(1), dels.Load())

			mr.SetTime(time.UnixMilli(deadline))
			_, err := deleteScript.Run(t.Context(), client, keys, "", "T.DEL", "x", deadline, reqID, deletePruneBatch).Result()
			require.ErrorContains(t, err, "PFEXPIRED")
			require.Equal(t, int32(1), dels.Load(), "a replay at the deadline must not delete again")
		})
	}
}

// TestStageScript_ExpiryFailureLeavesNoKey injects a failure into the real
// stage script right after the reserve: the reserved key must be removed.
func TestStageScript_ExpiryFailureLeavesNoKey(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	src := strings.Replace(stageScriptSource, "redis.pcall('PEXPIRE'", "redis.pcall('NOSUCHCOMMAND'", 1)
	require.NotEqual(t, stageScriptSource, src)

	// SET stands in for the module's reserve command: it creates the key.
	keys := []string{"staging", "registry", "registry-deadlines"}
	_, err := goredis.NewScript(src).Run(t.Context(), client, keys, int64(60000), farDeadline, "id-1", deletePruneBatch, "SET", "x").Result()
	require.Error(t, err)
	require.False(t, mr.Exists("staging"), "a staging key without expiry must not survive a failed Stage")

	_, err = stageScript.Run(t.Context(), client, keys, int64(60000), farDeadline, "id-2", deletePruneBatch, "SET", "x").Result()
	require.NoError(t, err)
	require.True(t, mr.Exists("staging"))
	require.Equal(t, time.Minute, mr.TTL("staging"))
}

// TestStageScript_ReplayCannotRecreateLostStaging creates a staging key, loses
// it (expiry or eviction) and replays the same stage request, as a delayed
// first attempt of a retried request would: the key must not be recreated,
// and a replay past the deadline is rejected outright.
func TestStageScript_ReplayCannotRecreateLostStaging(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	keys := []string{"staging", "registry", "registry-deadlines"}
	start := time.Now().Truncate(time.Millisecond)
	mr.SetTime(start)
	deadline := start.Add(time.Minute).UnixMilli()

	_, err := stageScript.Run(t.Context(), client, keys, int64(60000), deadline, "id-1", deletePruneBatch, "SET", "x").Result()
	require.NoError(t, err)
	mr.Del("staging") // lost mid-rebuild

	_, err = stageScript.Run(t.Context(), client, keys, int64(60000), deadline, "id-1", deletePruneBatch, "SET", "x").Result()
	require.ErrorContains(t, err, "PFREPLAY")
	require.False(t, mr.Exists("staging"), "a replayed stage request must not recreate the staging key")

	mr.SetTime(time.UnixMilli(deadline))
	_, err = stageScript.Run(t.Context(), client, keys, int64(60000), deadline, "id-1", deletePruneBatch, "SET", "x").Result()
	require.ErrorContains(t, err, "PFEXPIRED")
	require.False(t, mr.Exists("staging"))
	require.Zero(t, mr.TTL("registry"), "the registry must not expire")
}

// TestStaging_Commit_LostReplyAndLostMarkerIsIndeterminate promotes through
// the script, drops the marker (as eviction would) and replays the commit as
// a client retry: the outcome must be indeterminate, not a definite failure.
func TestStaging_Commit_LostReplyAndLostMarkerIsIndeterminate(t *testing.T) {
	t.Parallel()
	mr, st := stagedInternal(t)

	keys := commitKeys(st)
	n, err := commitScript.Run(t.Context(), st.core.client, keys, int64(60000), "0").Int()
	require.NoError(t, err)
	require.Equal(t, 1, n)
	mr.Del(st.markerKey())

	require.ErrorIs(t, st.Commit(t.Context()), probfilter.ErrCommitIndeterminate)
	got, err := mr.Get("t:f")
	require.NoError(t, err)
	require.Equal(t, "new", got)
}

// TestCore_TimeComesFromOwningShard checks that delete and stage deadlines use
// keyed scripts (routed to the shard owning the filter) rather than a keyless
// TIME command.
func TestCore_TimeComesFromOwningShard(t *testing.T) {
	t.Parallel()
	mr, client, core, _ := newDeleteCore(t)
	var keyless atomic.Int32
	client.AddHook(timeSpyHook{keyless: &keyless})

	_, err := core.Delete(t.Context(), "T.DEL", "x")
	require.NoError(t, err)
	require.NoError(t, mr.Set("t:f", "old"))
	_, err = core.Stage(t.Context())
	require.NoError(t, err)
	require.Zero(t, keyless.Load(), "no keyless TIME may be issued")
}

type timeSpyHook struct{ keyless *atomic.Int32 }

func (timeSpyHook) DialHook(next goredis.DialHook) goredis.DialHook { return next }

func (h timeSpyHook) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return func(ctx context.Context, cmd goredis.Cmder) error {
		if strings.EqualFold(cmd.Name(), "time") {
			h.keyless.Add(1)
		}
		return next(ctx, cmd)
	}
}

func (h timeSpyHook) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []goredis.Cmder) error {
		for _, cmd := range cmds {
			if strings.EqualFold(cmd.Name(), "time") {
				h.keyless.Add(1)
			}
		}
		return next(ctx, cmds)
	}
}

// TestStaging_Commit_ReplayAfterPartialAttemptAdvancesGeneration fails a
// commit attempt after it advanced the generation but before the rename,
// captures a delete request under that generation (as another process
// would), then replays the commit: the promoting attempt must advance the
// generation again so the captured delete is rejected.
func TestStaging_Commit_ReplayAfterPartialAttemptAdvancesGeneration(t *testing.T) {
	t.Parallel()
	mr, client, core, dels := newDeleteCore(t)
	require.NoError(t, mr.Set("t:f", "old"))
	st, err := core.Stage(t.Context())
	require.NoError(t, err)
	require.NoError(t, mr.Set(st.Key(), "new"))

	partial := strings.Replace(commitScriptSource, "redis.call('PERSIST'", "do return redis.error_reply('ERR injected') end --", 1)
	require.NotEqual(t, commitScriptSource, partial)
	keys := commitKeys(st)
	_, err = goredis.NewScript(partial).Run(t.Context(), client, keys, int64(60000), "0").Result()
	require.ErrorContains(t, err, "injected")

	observed, _, err := core.generationAndTime(t.Context())
	require.NoError(t, err)

	require.NoError(t, st.Commit(t.Context()))
	got, err := mr.Get("t:f")
	require.NoError(t, err)
	require.Equal(t, "new", got)

	delKeys := deleteKeys(core)
	reply, err := deleteScript.Run(t.Context(), client, delKeys, observed, "T.DEL", "x", farDeadline, "captured", deletePruneBatch).Result()
	require.NoError(t, err)
	require.Equal(t, int64(staleGeneration), reply, "a delete captured during the failed attempt must not reach the promoted filter")
	require.Zero(t, dels.Load())
}

func commitKeys(st *Staging) []string {
	return []string{st.core.filterKey, st.live.filterKey, st.markerKey(), st.live.genKey, st.live.leaseKey, st.live.committedKey, st.live.readyKey}
}
