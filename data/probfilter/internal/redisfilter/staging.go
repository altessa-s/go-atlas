// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisfilter

import (
	"context"
	"errors"
	"iter"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/data/probfilter"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// StagingTTL bounds the lifetime of a staging key, so a process that dies
// mid-rebuild cannot leak it. The TTL is refreshed after every staging batch,
// so it only needs to cover the longest pause between two batches; a staging
// key that expires anyway makes the rebuild fail instead of committing a
// partial filter.
const StagingTTL = time.Hour

// commitMarkerTTL is how long the commit marker outlives a commit; it only
// needs to cover the reconciliation that follows a lost commit reply.
const commitMarkerTTL = 10 * time.Minute

// errStagingMissing reports a commit that found neither the staging key nor
// the commit marker: either nothing was promoted (the staging key expired or
// was deleted) or an earlier attempt promoted it and the marker is gone.
var errStagingMissing = errors.New("neither staging filter nor commit marker found")

// stageScript reserves the staging filter and sets its TTL atomically, so a
// reserved staging key can never exist without an expiry; if the expiry
// cannot be set, the just-reserved key is deleted again.
//
// Each staging id may be created at most once: the request carries a
// server-time deadline, is rejected at or past it, and records its id in a
// per-filter registry (hash plus deadline sorted set, no TTL) that is pruned
// only past the deadline. A delayed replay of the request (for example a
// client retry whose first attempt executes late) therefore cannot recreate
// a staging key that expired or was evicted mid-rebuild, which would let the
// rebuild commit an incomplete filter.
//
// KEYS[1] staging key; KEYS[2] registry hash; KEYS[3] registry deadlines;
// ARGV[1] TTL ms; ARGV[2] deadline (server unix ms); ARGV[3] staging id;
// ARGV[4] prune batch size; ARGV[5] reserve command; ARGV[6:] reserve args.
var stageScript = redis.NewScript(stageScriptSource)

const stageScriptSource = `
local t = redis.call('TIME')
local now = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000)
if now >= tonumber(ARGV[2]) then
  return redis.error_reply('PFEXPIRED stage request past its deadline')
end
local old = redis.call('ZRANGEBYSCORE', KEYS[3], '-inf', now, 'LIMIT', 0, tonumber(ARGV[4]))
for _, id in ipairs(old) do
  redis.call('HDEL', KEYS[2], id)
  redis.call('ZREM', KEYS[3], id)
end
if redis.call('HEXISTS', KEYS[2], ARGV[3]) == 1 then
  return redis.error_reply('PFREPLAY staging filter already created')
end
redis.call('HSET', KEYS[2], ARGV[3], ARGV[2])
redis.call('ZADD', KEYS[3], ARGV[2], ARGV[3])
local args = {ARGV[5], KEYS[1]}
for i = 6, #ARGV do args[#args + 1] = ARGV[i] end
redis.call(unpack(args))
local ok = redis.pcall('PEXPIRE', KEYS[1], ARGV[1])
if type(ok) == 'table' and ok.err then
  redis.pcall('DEL', KEYS[1])
  return ok
end
return 1
`

// stageRequestWindow is how long after its creation a stage request may
// execute.
const stageRequestWindow = time.Minute

// commitScript promotes the staging filter: it strips the staging TTL, renames
// the staging key onto the live key and leaves a short-lived commit marker.
// Stripping the TTL first means a promoted live key can never carry it, even
// if a later step fails. Re-running it after a commit whose reply was lost
// (a client retry) finds the marker and reports success again.
// It first advances the generation (INCR), which fences deletes aimed at the
// replaced filter (see [Core.Delete]). Every execution that may promote
// advances it again — a retried or replayed attempt included — so a delete
// that observed the generation of a failed attempt cannot match the one
// under which a later attempt promotes. Advancing it before the rename means
// a failure to advance it prevents the promotion.
//
// A staging filter created under a rebuild lease (ticket != 0) promotes only
// while that lease still holds its ticket and no newer ticket was published:
// a rebuild whose lease expired cannot overwrite the newer snapshot of the
// rebuild that took over. Its ticket is recorded as the committed ticket
// before any promotion step, so a partially failed attempt still fences
// older tickets; same-ticket retries may resume.
// The ready marker (see [Core.RebuildCommitted]) is set last, so only an
// execution that completed the rename sets it.
// KEYS[1] staging key; KEYS[2] live key; KEYS[3] marker key; KEYS[4]
// generation key; KEYS[5] lease key; KEYS[6] committed-ticket key; KEYS[7]
// ready marker; ARGV[1] marker TTL ms; ARGV[2] ticket ("0": no lease).
// Returns 1 when promoted (now or by an earlier run), 0 when nothing exists to
// promote, -1 when superseded.
var commitScript = redis.NewScript(commitScriptSource)

const commitScriptSource = `
if redis.call('EXISTS', KEYS[1]) == 0 then
  return redis.call('EXISTS', KEYS[3])
end
if ARGV[2] ~= '0' then
  if redis.call('GET', KEYS[5]) ~= ARGV[2] then return -1 end
  local committed = tonumber(redis.call('GET', KEYS[6]) or '0')
  if committed > tonumber(ARGV[2]) then return -1 end
  redis.call('SET', KEYS[6], ARGV[2])
end
local gentype = redis.call('TYPE', KEYS[4]).ok
if gentype ~= 'none' and gentype ~= 'string' then
  return redis.error_reply('WRONGTYPE probfilter generation key holds a ' .. gentype)
end
redis.call('INCR', KEYS[4])
redis.call('PERSIST', KEYS[1])
redis.call('RENAME', KEYS[1], KEYS[2])
redis.call('SET', KEYS[3], '1', 'PX', ARGV[1])
redis.call('SET', KEYS[7], '1')
return 1
`

// Staging is a replacement filter reserved under a private staging key. It
// is populated while the live filter keeps serving and promoted with an
// atomic RENAME. It is used by a single rebuild goroutine.
type Staging struct {
	live *Core
	core *Core
	id   string // random id shared by the staging and marker keys
	// ticket is the rebuild lease ticket; 0 when staged without a lease.
	ticket int64
	// commit is the promotion script; a field so tests can inject failures.
	commit *redis.Script
}

// Stage reserves an empty replacement filter with reserveArgs under a fresh
// staging key in the same cluster hash slot as the live key. Reserve and a
// [StagingTTL] expiry run in one script, so the staging key never exists
// without a TTL; [Staging.Commit] strips it again. Staging batches use the
// non-creating batch command, so a vanished staging key fails the rebuild.
func (c *Core) Stage(ctx context.Context, reserveArgs ...any) (*Staging, error) {
	if c.keyErr != nil {
		return nil, c.keyErr
	}
	id, err := randomID()
	if err != nil {
		return nil, coreerrs.WrapOperation(err, c.opCreate)
	}
	key := metaKey(c.filterKey, "staging", id)

	staged := newCore(c.client, key, c.cmds, reserveArgs...)
	staged.autoCreate = false
	staged.afterBatch = func(ctx context.Context) error {
		if expireErr := c.client.PExpire(ctx, key, StagingTTL).Err(); expireErr != nil {
			return coreerrs.WrapOperation(expireErr, staged.opBatch)
		}
		return nil
	}

	// The owning shard's clock (keyed script), not an arbitrary node's.
	now, err := c.shardTime(ctx, key)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, c.opCreate)
	}
	deadline := now.Add(stageRequestWindow).UnixMilli()

	keys := []string{key, c.stagingsKey, c.stagingDeadlinesKey}
	argv := append([]any{StagingTTL.Milliseconds(), deadline, id, deletePruneBatch, c.cmds.Reserve}, reserveArgs...)
	if err := stageScript.Run(ctx, c.client, keys, argv...).Err(); err != nil {
		return nil, coreerrs.WrapOperation(err, c.opCreate)
	}
	return &Staging{live: c, core: staged, id: id, commit: commitScript}, nil
}

// Key returns the staging key.
func (s *Staging) Key() string {
	return s.core.filterKey
}

// AddBatch inserts values into the replacement filter and refreshes the
// staging key's TTL. It fails, rather than recreating the key, when the
// staging key has vanished.
func (s *Staging) AddBatch(ctx context.Context, values iter.Seq[string]) error {
	return s.core.AddBatch(ctx, values)
}

// Commit atomically replaces the live filter with the replacement and strips
// the staging TTL, in one script. It fails without renaming when ctx is
// already canceled or the staging key has vanished.
//
// When the script fails, Commit reconciles before reporting. Success needs
// positive evidence — the commit marker, which the script writes only after
// the rename. Every other failure is reported as an error wrapping
// [probfilter.ErrCommitIndeterminate]: with client retries an earlier attempt
// may still execute later, so even a server error reply does not prove that
// nothing will be promoted.
func (s *Staging) Commit(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	op := "promote Redis " + s.live.cmds.Label + " filter"
	marker := s.markerKey()
	keys := []string{s.core.filterKey, s.live.filterKey, marker, s.live.genKey, s.live.leaseKey, s.live.committedKey, s.live.readyKey}
	n, err := s.commit.Run(ctx, s.core.client, keys, commitMarkerTTL.Milliseconds(), strconv.FormatInt(s.ticket, 10)).Int()
	switch {
	case err == nil && n == 1:
		return nil
	case err == nil && n < 0:
		// Definite: the script renamed nothing, and the lease and committed
		// ticket only move on, so no retry of this commit can promote later.
		return coreerrs.WrapOperation(probfilter.ErrRebuildSuperseded, op)
	case err == nil:
		// Non-promotion cannot be established: a retried script may follow a
		// promotion whose reply and marker were both lost.
		return coreerrs.WrapOperation(errors.Join(probfilter.ErrCommitIndeterminate, errStagingMissing), op)
	}

	// Even a server error reply proves nothing about other attempts: the
	// client may have retried after a timeout while an earlier attempt is
	// still on its way. Only the marker, written after the rename, proves
	// the promotion; everything else is indeterminate, and the caller must
	// discard the staging key (making any late attempt harmless) or fence.
	committed, checkErr := s.core.client.Exists(context.WithoutCancel(ctx), marker).Result()
	if checkErr == nil && committed == 1 {
		return nil
	}
	return coreerrs.WrapOperation(errors.Join(probfilter.ErrCommitIndeterminate, err), op)
}

// Abort deletes the replacement filter.
func (s *Staging) Abort(ctx context.Context) error {
	return s.core.DeleteFilter(ctx)
}

// markerKey is the commit marker key of this staging filter.
func (s *Staging) markerKey() string {
	return metaKey(s.live.filterKey, "committed", s.id)
}
