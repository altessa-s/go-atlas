// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/data/internal/redisbase"
	"github.com/altessa-s/go-atlas/data/saga"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	sagaerrs "github.com/altessa-s/go-atlas/data/saga/errs"
)

// Hash field names and key suffixes used by the store.
const (
	hashFieldData    = "d" // serialized saga.Instance JSON; the 'v' version field is read only from Lua
	instanceKeyTag   = "i:"
	recoverableIndex = "index:recoverable"
)

// createScript inserts a brand-new instance hash and registers it in the
// recoverable index. Returns 1 on success, 0 when the instance already exists.
//
// KEYS[1]=instance hash, KEYS[2]=recoverable ZSET.
// ARGV: 1=json, 2=version, 3=score, 4=member(id), 5=indexed('1'/'0'), 6=ttlMs.
var createScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 1 then return 0 end
redis.call('HSET', KEYS[1], 'd', ARGV[1], 'v', ARGV[2])
if tonumber(ARGV[6]) > 0 then redis.call('PEXPIRE', KEYS[1], ARGV[6]) end
if ARGV[5] == '1' then redis.call('ZADD', KEYS[2], ARGV[3], ARGV[4])
else redis.call('ZREM', KEYS[2], ARGV[4]) end
return 1
`)

// updateScript performs a version-checked write and updates the recoverable
// index. Returns 1 on success, 0 on version mismatch, -1 when the instance is
// absent.
//
// KEYS[1]=instance hash, KEYS[2]=recoverable ZSET.
// ARGV: 1=expectedVersion, 2=json, 3=newVersion, 4=score, 5=member(id),
//
//	6=indexed('1'/'0'), 7=ttlMs.
var updateScript = redis.NewScript(`
local v = redis.call('HGET', KEYS[1], 'v')
if not v then return -1 end
if tonumber(v) ~= tonumber(ARGV[1]) then return 0 end
redis.call('HSET', KEYS[1], 'd', ARGV[2], 'v', ARGV[3])
if tonumber(ARGV[7]) > 0 then redis.call('PEXPIRE', KEYS[1], ARGV[7]) end
if ARGV[6] == '1' then redis.call('ZADD', KEYS[2], ARGV[4], ARGV[5])
else redis.call('ZREM', KEYS[2], ARGV[5]) end
return 1
`)

// deleteScript removes the instance hash and its recoverable-index entry.
//
// KEYS[1]=instance hash, KEYS[2]=recoverable ZSET. ARGV[1]=member(id).
var deleteScript = redis.NewScript(`
redis.call('DEL', KEYS[1])
redis.call('ZREM', KEYS[2], ARGV[1])
return 1
`)

// Store is a durable [saga.Store] backed by Redis. Each instance is a hash
// keyed by its ID holding the serialized payload and a monotonically
// increasing version field used as the optimistic-concurrency token, so two
// coordinators cannot advance the same instance — the loser's Update fails
// with [sagaerrs.ErrVersionConflict].
//
// Because Redis is not query-capable, recoverable instances are tracked in a
// sorted set scored by recover-eligibility time: a timed-out RUNNING instance
// scores its deadline, a COMPENSATING instance scores 0 (always due), and any
// other instance is absent. FetchRecoverable is a ZRANGEBYSCORE over that set.
type Store struct {
	redisbase.Base
	opts *options
}

var _ saga.Store = (*Store)(nil)

// New creates a Store with the given Redis client. It panics if client is nil.
//
// Example:
//
//	store := redis.New(rdb, redis.WithKeyPrefix("saga:"))
func New(client redis.UniversalClient, opt ...Option) *Store {
	opts := newOptions(opt...)
	return &Store{
		Base: redisbase.NewBase(client, opts.keyPrefix),
		opts: opts,
	}
}

// Create inserts a new instance, returning [sagaerrs.ErrInstanceExists] when one
// with the same ID already exists.
func (s *Store) Create(ctx context.Context, inst *saga.Instance) error {
	data, err := json.Marshal(inst)
	if err != nil {
		return coreerrs.WrapOperation(err, "marshal saga instance")
	}
	score, indexed := recoverScore(inst)

	res, err := createScript.Run(ctx, s.Client(),
		[]string{s.instanceKey(inst.ID), s.indexKey()},
		data, inst.Version, score, inst.ID, boolArg(indexed), s.ttlMs(),
	).Int64()
	if err != nil {
		return coreerrs.WrapOperation(err, "create saga instance in Redis")
	}
	if res == 0 {
		return sagaerrs.ErrInstanceExists
	}
	return nil
}

// Get loads an instance by ID, returning [sagaerrs.ErrInstanceNotFound] when
// absent.
func (s *Store) Get(ctx context.Context, id string) (*saga.Instance, error) {
	data, err := s.Client().HGet(ctx, s.instanceKey(id), hashFieldData).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, sagaerrs.ErrInstanceNotFound
		}
		return nil, coreerrs.WrapOperation(err, "get saga instance from Redis")
	}
	return decodeInstance(data)
}

// Update overwrites the instance using a version-checked write. A stale
// inst.Version yields [sagaerrs.ErrVersionConflict]; a missing instance yields
// [sagaerrs.ErrInstanceNotFound]. The new version is written back into
// inst.Version on success.
func (s *Store) Update(ctx context.Context, inst *saga.Instance) error {
	newVersion := inst.Version + 1

	next := inst.Clone()
	next.Version = newVersion
	data, err := json.Marshal(next)
	if err != nil {
		return coreerrs.WrapOperation(err, "marshal saga instance")
	}
	score, indexed := recoverScore(next)

	res, err := updateScript.Run(ctx, s.Client(),
		[]string{s.instanceKey(inst.ID), s.indexKey()},
		inst.Version, data, newVersion, score, inst.ID, boolArg(indexed), s.ttlMs(),
	).Int64()
	if err != nil {
		return coreerrs.WrapOperation(err, "update saga instance in Redis")
	}
	switch res {
	case 1:
		inst.Version = newVersion
		return nil
	case 0:
		return sagaerrs.ErrVersionConflict
	default: // -1
		return sagaerrs.ErrInstanceNotFound
	}
}

// FetchRecoverable returns up to limit non-terminal instances that are
// mid-compensation or past their deadline. A non-positive limit means no cap.
func (s *Store) FetchRecoverable(ctx context.Context, now time.Time, limit int) ([]*saga.Instance, error) {
	rangeBy := &redis.ZRangeBy{Min: "-inf", Max: strconv.FormatInt(now.Unix(), 10)}
	if limit > 0 {
		rangeBy.Count = int64(limit)
	}

	ids, err := s.Client().ZRangeByScore(ctx, s.indexKey(), rangeBy).Result()
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "fetch recoverable saga ids from Redis")
	}
	if len(ids) == 0 {
		return nil, nil
	}

	pipe := s.Client().Pipeline()
	cmds := make([]*redis.StringCmd, len(ids))
	for i, id := range ids {
		cmds[i] = pipe.HGet(ctx, s.instanceKey(id), hashFieldData)
	}
	// Exec returns the first command error (including redis.Nil); each command
	// is inspected individually below, so its aggregate error is ignored here.
	_, _ = pipe.Exec(ctx)

	out := make([]*saga.Instance, 0, len(ids))
	var stale []any
	for i, cmd := range cmds {
		data, cErr := cmd.Bytes()
		if cErr != nil {
			if errors.Is(cErr, redis.Nil) {
				stale = append(stale, ids[i]) // Key gone since it was indexed.
				continue
			}
			return nil, coreerrs.WrapOperation(cErr, "read recoverable saga instance from Redis")
		}
		inst, dErr := decodeInstance(data)
		if dErr != nil {
			return nil, dErr
		}
		out = append(out, inst)
	}

	if len(stale) > 0 {
		_ = s.Client().ZRem(ctx, s.indexKey(), stale...).Err() // Best-effort cleanup.
	}
	return out, nil
}

// Delete removes an instance and its recoverable-index entry. Deleting a
// missing instance is not an error.
func (s *Store) Delete(ctx context.Context, id string) error {
	if err := deleteScript.Run(ctx, s.Client(),
		[]string{s.instanceKey(id), s.indexKey()}, id,
	).Err(); err != nil {
		return coreerrs.WrapOperation(err, "delete saga instance from Redis")
	}
	return nil
}

// instanceKey builds the prefixed hash key for an instance ID.
func (s *Store) instanceKey(id string) string { return s.Key(instanceKeyTag + id) }

// indexKey builds the prefixed key of the recoverable sorted set.
func (s *Store) indexKey() string { return s.Key(recoverableIndex) }

// ttlMs renders the configured TTL as a millisecond string ("0" = persist).
func (s *Store) ttlMs() string {
	if s.opts.ttl <= 0 {
		return "0"
	}
	return strconv.FormatInt(s.opts.ttl.Milliseconds(), 10)
}

// recoverScore reports the recoverable-index score for an instance and whether
// it should be indexed at all. A COMPENSATING instance is always due (score 0);
// a RUNNING instance with a deadline is due at that deadline; everything else
// (terminal, or RUNNING without a deadline) is not indexed.
func recoverScore(inst *saga.Instance) (score float64, indexed bool) {
	switch {
	case inst.Status.IsTerminal():
		return 0, false
	case inst.Status == saga.StatusCompensating:
		return 0, true
	case inst.Status == saga.StatusRunning && !inst.Deadline.IsZero():
		return float64(inst.Deadline.Unix()), true
	default:
		return 0, false
	}
}

// boolArg renders a bool as the "1"/"0" string the Lua scripts expect.
func boolArg(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// decodeInstance unmarshals a stored instance payload.
func decodeInstance(data []byte) (*saga.Instance, error) {
	var inst saga.Instance
	if err := json.Unmarshal(data, &inst); err != nil {
		return nil, coreerrs.WrapOperation(err, "unmarshal saga instance")
	}
	return &inst, nil
}
