// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisfilter

import (
	"context"
	"strconv"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/data/probfilter"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// journaledInsertScript runs an insert against the live filter and, while a
// rebuild lease exists, appends the inserted values to the rebuild journal in
// the same atomic step. Every process sharing the filter journals this way,
// so the rebuild replays inserts it would otherwise lose when it renames its
// replacement over the live key. An insert error is returned unchanged and
// journals nothing.
//
// The journal carries no TTL: losing it would silently drop acknowledged
// inserts from the replacement, and a TTL would also make it evictable under
// the volatile-* policies the filter otherwise tolerates. It is dropped when
// a rebuild acquires or releases the lease and by the commit that replays it;
// one left by a crashed rebuild stops growing once its lease expires.
//
// KEYS[1] live filter; KEYS[2] rebuild lease; KEYS[3] journal; ARGV[1]
// number of trailing values to journal; ARGV[2] insert command; ARGV[3:]
// insert arguments after the key.
var journaledInsertScript = redis.NewScript(`
local args = {ARGV[2], KEYS[1]}
for i = 3, #ARGV do args[#args + 1] = ARGV[i] end
local r = redis.pcall(unpack(args))
if type(r) == 'table' and r.err then return r end
if redis.call('EXISTS', KEYS[2]) == 1 then
  local n = tonumber(ARGV[1])
  redis.call('RPUSH', KEYS[3], unpack(ARGV, #ARGV - n + 1, #ARGV))
end
return r
`)

// drainJournalScript moves the oldest journaled values into the staging
// filter while the lease still holds the rebuild's ticket: it reads a batch,
// inserts it into the staging key, refreshes the staging TTL and only then
// trims the batch from the journal — all in one atomic step. A superseded
// rebuild therefore never consumes its successor's journal, and a client
// retry after a lost reply moves the next batch instead of losing one.
//
// KEYS[1] rebuild lease; KEYS[2] journal; KEYS[3] staging filter; ARGV[1]
// ticket; ARGV[2] batch size; ARGV[3] staging TTL ms; ARGV[4] batch insert
// command; ARGV[5:] its tokens. Returns the number of values moved, 0 when
// the journal is empty, -1 when the lease no longer holds the ticket. A
// rejected value (an error item, e.g. a full non-scaling filter) fails the
// script before the trim, so the batch stays journaled and the rebuild fails.
var drainJournalScript = redis.NewScript(replyCheckLua + `
if redis.call('GET', KEYS[1]) ~= ARGV[1] then return -1 end
local items = redis.call('LRANGE', KEYS[2], 0, tonumber(ARGV[2]) - 1)
if #items == 0 then return 0 end
local args = {ARGV[4], KEYS[3]}
for i = 5, #ARGV do args[#args + 1] = ARGV[i] end
for _, v in ipairs(items) do args[#args + 1] = v end
local bad = checked(redis.call(unpack(args)), #items)
if bad then return bad end
redis.call('PEXPIRE', KEYS[3], ARGV[3])
redis.call('LTRIM', KEYS[2], #items, -1)
return #items
`)

// replyCheckLua defines checked(reply, n), which returns an error reply
// unless reply is an array of n items none of which is an error. Bloom's
// "already present" item (0 / false) is a success.
const replyCheckLua = `
local function checked(r, want)
  local got = type(r) == 'table' and #r or 0
  if type(r) ~= 'table' or got ~= want then
    return redis.error_reply('PFREPLAY journal replay returned ' .. got .. ' replies for ' .. want .. ' values')
  end
  for _, e in ipairs(r) do
    if type(e) == 'table' and e.err then return redis.error_reply('PFREPLAY journal replay rejected a value: ' .. e.err) end
  end
  return nil
end
`

// drainBatch is the number of journaled values drainJournalScript moves per
// call.
const drainBatch = 500

// insertJournaled runs the insert command args (command, live key, arguments)
// through journaledInsertScript; the last values arguments are the inserted
// values.
func (c *Core) insertJournaled(ctx context.Context, args []any, values int) (any, error) {
	argv := make([]any, 0, len(args)+1)
	argv = append(argv, values, args[0])
	argv = append(argv, args[2:]...)
	return journaledInsertScript.Run(ctx, c.client, []string{c.filterKey, c.leaseKey, c.journalKey}, argv...).Result()
}

// drainJournal moves the journaled inserts into the staging filter in
// batches, so the commit script only replays the short tail journaled after
// the last batch. It moves at most the values journaled when it starts, so
// writers that keep appending cannot hold the rebuild back; the commit
// replays what they add meanwhile. A staging filter created without a lease
// has no journal of its own and leaves it alone. A lease lost meanwhile fails
// the rebuild with [probfilter.ErrRebuildSuperseded], before anything is
// promoted.
func (s *Staging) drainJournal(ctx context.Context) error {
	if s.ticket == 0 {
		return nil
	}
	op := "drain Redis " + s.live.cmds.Label + " filter rebuild journal"
	keys := []string{s.live.leaseKey, s.live.journalKey, s.core.filterKey}
	argv := []any{strconv.FormatInt(s.ticket, 10), drainBatch, StagingTTL.Milliseconds(), s.live.cmds.AddBatch}
	for _, token := range s.live.cmds.BatchTokens {
		argv = append(argv, token)
	}
	budget, err := s.core.client.LLen(ctx, s.live.journalKey).Result()
	if err != nil {
		return coreerrs.WrapOperation(err, op)
	}
	for budget > 0 {
		n, err := drainJournalScript.Run(ctx, s.core.client, keys, argv...).Int64()
		switch {
		case err != nil:
			return coreerrs.WrapOperation(err, op)
		case n < 0:
			return coreerrs.WrapOperation(probfilter.ErrRebuildSuperseded, op)
		case n == 0:
			return nil
		}
		budget -= n
	}
	return nil
}

// journalReplayArgs returns the commit script arguments that replay the rest
// of the journal onto the staging filter: the batch insert command and its
// tokens, or an empty command when the filter does not journal inserts or the
// staging filter was created without a lease (the journal belongs to the
// lease holder).
func (s *Staging) journalReplayArgs() []any {
	if !s.live.journalAdds || s.ticket == 0 {
		return []any{""}
	}
	args := make([]any, 0, 1+len(s.live.cmds.BatchTokens))
	args = append(args, s.live.cmds.AddBatch)
	for _, token := range s.live.cmds.BatchTokens {
		args = append(args, token)
	}
	return args
}
