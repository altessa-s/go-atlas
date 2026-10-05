// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisfilter

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// staleGeneration is the reply of deleteScript when the filter was replaced
// since the caller read its generation token.
const staleGeneration = -2

// deleteRequestWindow is how long after its creation a delete request may
// execute; a retry arriving later is rejected without deleting.
const deleteRequestWindow = time.Minute

// deletePruneBatch bounds how many outdated request records one delete
// prunes, keeping every delete script short.
const deletePruneBatch = 100

// ErrStaleGeneration is returned by [Core.Delete] when a rebuild replaced the
// filter between reading its generation and executing the delete; nothing
// was deleted.
var ErrStaleGeneration = errors.New("redis filter replaced during delete")

// deleteScript deletes a value at most once, and only from the filter
// generation the caller observed:
//
//   - every committed rebuild writes a new token to the generation key, so a
//     delete delayed past a rebuild finds a different token and does nothing
//     instead of removing a colliding member of the new filter;
//   - every request carries an execution deadline in server time; a request
//     at or past it is rejected before anything else, and records are pruned
//     only at or past their deadline, so a request can never outlive its
//     record;
//   - before the delete runs, a "pending" record of the request is stored in
//     a per-filter hash without TTL (so no eviction policy limited to keys
//     with an expiry can drop it) and replaced by the outcome afterwards; a
//     client retry of the same request (e.g. after a lost reply) returns the
//     outcome — or, if the outcome could not be recorded, an error — instead
//     of removing a second, colliding fingerprint. Records are pruned only
//     once their request is past its deadline, when no retry can run anyway.
//
// KEYS[1] live key; KEYS[2] generation key; KEYS[3] request-record hash;
// KEYS[4] request-deadline sorted set; ARGV[1] expected token ("" for a
// never-rebuilt filter); ARGV[2] delete command; ARGV[3] value; ARGV[4]
// deadline (server unix ms); ARGV[5] request id; ARGV[6] prune batch size.
var deleteScript = redis.NewScript(deleteScriptSource)

const deleteScriptSource = `
local t = redis.call('TIME')
local now = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000)
if now >= tonumber(ARGV[4]) then
  return redis.error_reply('PFEXPIRED delete request past its deadline')
end
local old = redis.call('ZRANGEBYSCORE', KEYS[4], '-inf', now, 'LIMIT', 0, tonumber(ARGV[6]))
for _, id in ipairs(old) do
  redis.call('HDEL', KEYS[3], id)
  redis.call('ZREM', KEYS[4], id)
end
local done = redis.call('HGET', KEYS[3], ARGV[5])
if done == 'pending' then return redis.error_reply('PFPENDING delete outcome unknown') end
if done then return tonumber(done) end
local cur = redis.call('GET', KEYS[2])
if cur == false then cur = '' end
if cur ~= ARGV[1] then return -2 end
redis.call('HSET', KEYS[3], ARGV[5], 'pending')
redis.call('ZADD', KEYS[4], ARGV[4], ARGV[5])
local r = redis.pcall(ARGV[2], KEYS[1], ARGV[3])
if type(r) == 'table' and r.err then
  redis.call('HDEL', KEYS[3], ARGV[5])
  redis.call('ZREM', KEYS[4], ARGV[5])
  return r
end
redis.call('HSET', KEYS[3], ARGV[5], tostring(r))
return r
`

// ErrDeleteIndeterminate is returned by [Core.Delete] when a retried delete
// request finds that an earlier attempt started deleting but could not
// record its outcome, or arrives after its execution deadline; the value may
// or may not have been deleted. It is also returned for a request that never
// ran but was delayed past its deadline.
var ErrDeleteIndeterminate = errors.New("redis filter delete outcome unknown")

// Delete runs the delete command (e.g. "CF.DEL") for value at most once,
// against the filter generation that is live when Delete starts, returning
// the command's raw reply. The generation token is read fresh for every
// delete; if a rebuild replaces the filter before the delete executes, the
// delete does nothing and Delete returns [ErrStaleGeneration] — it is never
// retried against the new filter, which may not hold value and could lose a
// colliding member.
func (c *Core) Delete(ctx context.Context, cmd, value string) (any, error) {
	if c.keyErr != nil {
		return nil, c.keyErr
	}

	// One keyed script reads the generation token and the clock of the shard
	// that owns the filter (a keyless TIME could be answered by another
	// Cluster node), anchoring the request's execution deadline.
	token, now, err := c.generationAndTime(ctx)
	if err != nil {
		return nil, err
	}
	deadline := now.Add(deleteRequestWindow).UnixMilli()

	id, err := randomID()
	if err != nil {
		return nil, err
	}
	keys := []string{c.filterKey, c.genKey, c.deletesKey, c.deadlinesKey}
	reply, err := deleteScript.Run(ctx, c.client, keys, token, cmd, value, deadline, id, deletePruneBatch).Result()
	if err != nil {
		if strings.HasPrefix(err.Error(), "PFPENDING") || strings.HasPrefix(err.Error(), "PFEXPIRED") {
			return nil, errors.Join(ErrDeleteIndeterminate, err)
		}
		return nil, err
	}
	if n, ok := reply.(int64); ok && n == staleGeneration {
		return nil, ErrStaleGeneration
	}
	return reply, nil
}

// Reply lengths of generationTimeScript ({token, secs, micros}) and
// shardTimeScript ({secs, micros}).
const (
	generationTimeReplyLen = 3
	shardTimeReplyLen      = 2
)

// generationTimeScript returns the generation token ("" when missing) and the
// server time, both from the shard owning KEYS[1] (the generation key).
var generationTimeScript = redis.NewScript(`
local g = redis.call('GET', KEYS[1])
if not g then g = '' end
local t = redis.call('TIME')
return {g, t[1], t[2]}
`)

// shardTimeScript returns the time of the shard owning KEYS[1].
var shardTimeScript = redis.NewScript(`
local t = redis.call('TIME')
return {t[1], t[2]}
`)

// generationAndTime reads the generation token and the owning shard's time.
func (c *Core) generationAndTime(ctx context.Context) (string, time.Time, error) {
	reply, err := generationTimeScript.Run(ctx, c.client, []string{c.genKey}).Slice()
	if err != nil {
		return "", time.Time{}, coreerrs.WrapOperation(err, "read Redis "+c.cmds.Label+" filter generation")
	}
	if len(reply) != generationTimeReplyLen {
		return "", time.Time{}, fmt.Errorf("read Redis filter generation: unexpected reply %v", reply)
	}
	token, _ := reply[0].(string)
	now, err := parseTime(reply[1], reply[2])
	return token, now, err
}

// shardTime returns the time of the Redis shard owning key.
func (c *Core) shardTime(ctx context.Context, key string) (time.Time, error) {
	reply, err := shardTimeScript.Run(ctx, c.client, []string{key}).Slice()
	if err != nil {
		return time.Time{}, coreerrs.WrapOperation(err, "read Redis server time")
	}
	if len(reply) != shardTimeReplyLen {
		return time.Time{}, fmt.Errorf("read Redis server time: unexpected reply %v", reply)
	}
	return parseTime(reply[0], reply[1])
}

// parseTime converts TIME's seconds and microseconds replies.
func parseTime(secs, micros any) (time.Time, error) {
	s, err := ToInt64(secs)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse Redis time: %w", err)
	}
	us, err := ToInt64(micros)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse Redis time: %w", err)
	}
	return time.Unix(s, us*int64(time.Microsecond)), nil
}
