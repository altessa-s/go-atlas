// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/internal/redis/redisbase"
	"github.com/altessa-s/go-atlas/reliability/saga"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// Hash field names and key suffixes used by the store.
const (
	hashFieldData    = "d" // serialized saga.Instance JSON
	hashFieldVersion = "v" // version, the compare-and-set token
	instanceKeyTag   = "i:"
	recoverableIndex = "index:recoverable"
	terminalIndex    = "index:terminal"
)

// ErrInvalidID is returned, before any network I/O, for an instance ID that is
// not valid UTF-8. The instance is stored as JSON, which would rewrite such an
// ID, so a later Update would address another instance's hash.
var ErrInvalidID = errors.New("saga instance id is not valid UTF-8")

// createScript inserts a brand-new instance hash and registers it in the
// recoverable index (or, when terminal, the terminal index). Returns 1 on
// success, 0 when the instance already exists.
//
// KEYS[1]=instance hash, KEYS[2]=recoverable ZSET, KEYS[3]=terminal ZSET.
// ARGV: 1=json, 2=version, 3=score, 4=member(id), 5=indexed('1'/'0'), 6=ttlMs,
// 7=terminal('1'/'0'), 8=terminalScore, 9=trimBefore (terminal-index entries
// scored below it belong to expired hashes and are dropped; 0 = none).
var createScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 1 then return 0 end
redis.call('HSET', KEYS[1], 'd', ARGV[1], 'v', ARGV[2])
if ARGV[7] == '1' then
  if tonumber(ARGV[6]) > 0 then redis.call('PEXPIRE', KEYS[1], ARGV[6]) end
  redis.call('ZADD', KEYS[3], ARGV[8], ARGV[4])
  if tonumber(ARGV[9]) > 0 then redis.call('ZREMRANGEBYSCORE', KEYS[3], '-inf', '(' .. ARGV[9]) end
end
if ARGV[5] == '1' then redis.call('ZADD', KEYS[2], ARGV[3], ARGV[4])
else redis.call('ZREM', KEYS[2], ARGV[4]) end
return 1
`)

// updateScript performs a version-checked write and updates the indexes. Only a
// terminal instance expires (ARGV[7] ttl); a live one must not vanish
// without compensation, so its expiry is cleared. Returns 1 on success, 0 on
// version mismatch, -1 when the instance is absent.
//
// KEYS[1]=instance hash, KEYS[2]=recoverable ZSET, KEYS[3]=terminal ZSET.
// ARGV: 1=expectedVersion, 2=json, 3=newVersion, 4=score, 5=member(id),
//
//	6=indexed('1'/'0'), 7=ttlMs, 8=terminal('1'/'0'), 9=terminalScore, 10=trimBefore.
var updateScript = redis.NewScript(`
local v = redis.call('HGET', KEYS[1], 'v')
if not v then return -1 end
if tonumber(v) ~= tonumber(ARGV[1]) then return 0 end
redis.call('HSET', KEYS[1], 'd', ARGV[2], 'v', ARGV[3])
if ARGV[8] == '1' then
  if tonumber(ARGV[7]) > 0 then redis.call('PEXPIRE', KEYS[1], ARGV[7]) end
  redis.call('ZADD', KEYS[3], ARGV[9], ARGV[5])
  if tonumber(ARGV[10]) > 0 then redis.call('ZREMRANGEBYSCORE', KEYS[3], '-inf', '(' .. ARGV[10]) end
else
  redis.call('PERSIST', KEYS[1])
end
if ARGV[6] == '1' then redis.call('ZADD', KEYS[2], ARGV[4], ARGV[5])
else redis.call('ZREM', KEYS[2], ARGV[5]) end
return 1
`)

// deleteScript removes the instance hash and its index entries.
//
// KEYS[1]=instance hash, KEYS[2]=recoverable ZSET, KEYS[3]=terminal ZSET.
// ARGV[1]=member(id).
var deleteScript = redis.NewScript(`
redis.call('DEL', KEYS[1])
redis.call('ZREM', KEYS[2], ARGV[1])
redis.call('ZREM', KEYS[3], ARGV[1])
return 1
`)

// deleteEligibleScript removes an instance for retention only while it is still
// the record the caller examined: listed in the terminal index below the cutoff,
// at the observed version and, when given, of the definition. Returns 1 when it
// deleted, 0 otherwise.
//
// KEYS[1]=instance hash, KEYS[2]=recoverable ZSET, KEYS[3]=terminal ZSET.
// ARGV: 1=member(id), 2=version, 3=cutoff (exclusive), 4=definition (” = any).
var deleteEligibleScript = redis.NewScript(`
local score = redis.call('ZSCORE', KEYS[3], ARGV[1])
if not score or tonumber(score) >= tonumber(ARGV[3]) then return 0 end
local v = redis.call('HGET', KEYS[1], 'v')
if not v or tonumber(v) ~= tonumber(ARGV[2]) then return 0 end
if ARGV[4] ~= '' then
  local d = redis.call('HGET', KEYS[1], 'd')
  local ok, inst = pcall(cjson.decode, d)
  if not ok or type(inst) ~= 'table' or inst.definition ~= ARGV[4] then return 0 end
end
redis.call('DEL', KEYS[1])
redis.call('ZREM', KEYS[2], ARGV[1])
redis.call('ZREM', KEYS[3], ARGV[1])
return 1
`)

// Store is a durable [saga.Storage] backed by Redis. Each instance is a hash
// keyed by its ID holding the serialized payload and a monotonically
// increasing version field used as the optimistic-concurrency token, so two
// coordinators cannot advance the same instance — the loser's Update fails
// with [saga.ErrVersionConflict].
//
// Because Redis is not query-capable, recoverable instances are tracked in a
// sorted set scored by recover-eligibility time: a timed-out RUNNING instance
// scores its deadline, a COMPENSATING instance scores 0 (always due), and any
// other instance is absent. FetchRecoverable reads the due ids page by page
// (ZRANGEBYSCORE ... LIMIT) and loads their instances. Terminal instances are
// tracked in a second sorted set scored by their last update, which
// DeleteTerminal pages the same way.
//
// Every write is a script over the instance hash and both indexes, so on Redis
// Cluster the three keys must share a hash slot: [New] rejects a cluster client
// whose key prefix has no hash tag.
type Store struct {
	redisbase.Base
	opts *options
}

var _ saga.Storage = (*Store)(nil)

// New creates a Store with the given Redis client. It panics if client is nil
// and fails with an error matching [coreerrs.ErrInvalidArgument] for a Redis
// Cluster client whose key prefix contains no hash tag ("{...}"): the scripts
// touch several keys, which Redis Cluster rejects across slots.
//
// Example:
//
//	store, err := redis.New(rdb, redis.WithKeyPrefix("saga:"))
func New(client redis.UniversalClient, opt ...Option) (*Store, error) {
	opts := newOptions(opt...)
	if _, cluster := client.(*redis.ClusterClient); cluster && !hasHashTag(opts.keyPrefix) {
		return nil, fmt.Errorf("%w: a Redis Cluster client needs a key prefix with a hash tag such as \"{saga}:\", got %q",
			coreerrs.ErrInvalidArgument, opts.keyPrefix)
	}
	return &Store{
		Base: redisbase.NewBase(client, opts.keyPrefix),
		opts: opts,
	}, nil
}

// hasHashTag reports whether prefix holds a complete, non-empty Redis hash tag,
// so every key built from it hashes by that tag whatever follows the prefix.
func hasHashTag(prefix string) bool {
	_, rest, found := strings.Cut(prefix, "{")
	if !found {
		return false
	}
	tag, _, found := strings.Cut(rest, "}")
	return found && tag != ""
}

// checkID rejects an ID that JSON would rewrite.
func checkID(id string) error {
	if !utf8.ValidString(id) {
		return errors.Join(saga.ErrInvalidStorageKey, ErrInvalidID)
	}
	return nil
}

// Create inserts a new instance, returning [saga.ErrInstanceExists] when one
// with the same ID already exists.
func (s *Store) Create(ctx context.Context, inst *saga.Instance) error {
	if err := checkID(inst.ID); err != nil {
		return err
	}
	data, err := json.Marshal(inst)
	if err != nil {
		return coreerrs.WrapOperation(err, "marshal saga instance")
	}
	score, indexed := recoverScore(inst)
	terminal, terminalScore := terminalArgs(inst)

	res, err := createScript.Run(ctx, s.Client(),
		[]string{s.instanceKey(inst.ID), s.indexKey(), s.terminalKey()},
		data, inst.Version, score, inst.ID, boolArg(indexed), s.ttlMs(), terminal, terminalScore, s.trimBefore(),
	).Int64()
	if err != nil {
		return coreerrs.WrapOperation(err, "create saga instance in Redis")
	}
	if res == 0 {
		return saga.ErrInstanceExists
	}
	return nil
}

// Get loads an instance by ID, returning [saga.ErrInstanceNotFound] when
// absent.
func (s *Store) Get(ctx context.Context, id string) (*saga.Instance, error) {
	if err := checkID(id); err != nil {
		return nil, err
	}
	data, err := s.Client().HGet(ctx, s.instanceKey(id), hashFieldData).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, saga.ErrInstanceNotFound
		}
		return nil, coreerrs.WrapOperation(err, "get saga instance from Redis")
	}
	return decodeInstance(data)
}

// Update overwrites the instance using a version-checked write. A stale
// inst.Version yields [saga.ErrVersionConflict]; a missing instance yields
// [saga.ErrInstanceNotFound]. The new version is written back into
// inst.Version on success.
func (s *Store) Update(ctx context.Context, inst *saga.Instance) error {
	if err := checkID(inst.ID); err != nil {
		return err
	}
	newVersion := inst.Version + 1

	// A shallow copy suffices: only Version changes, and the shared slices
	// are only read while encoding. inst.Version moves after a successful CAS.
	next := *inst
	next.Version = newVersion
	data, err := json.Marshal(&next)
	if err != nil {
		return coreerrs.WrapOperation(err, "marshal saga instance")
	}
	score, indexed := recoverScore(&next)
	terminal, terminalScore := terminalArgs(&next)

	res, err := updateScript.Run(ctx, s.Client(),
		[]string{s.instanceKey(inst.ID), s.indexKey(), s.terminalKey()},
		inst.Version, data, newVersion, score, inst.ID, boolArg(indexed), s.ttlMs(), terminal, terminalScore, s.trimBefore(),
	).Int64()
	if err != nil {
		return coreerrs.WrapOperation(err, "update saga instance in Redis")
	}
	switch res {
	case 1:
		inst.Version = newVersion
		return nil
	case 0:
		return saga.ErrVersionConflict
	default: // -1
		return saga.ErrInstanceNotFound
	}
}

// pageSize is the number of index members read and loaded per round trip.
const pageSize = 64

// member is an index entry: an instance ID and its score, as Redis rendered it.
type member struct{ id, score string }

// pageScript reads the next page of a sorted set strictly after a cursor. The
// cursor is the last (score, member) the caller saw, so entries removed before
// it between pages do not shift the scan (an offset would). When the cursor
// member itself is gone, the scan continues after its score and byte position
// among tied members, the order Redis keeps them in, so it always moves forward.
//
// KEYS[1]=ZSET. ARGV: 1=max score, 2=exclusive('1'/'0'), 3=cursor member (” for
// the first page), 4=cursor score, 5=count. Returns member, score pairs.
var pageScript = redis.NewScript(`
local function le(a, b)
  local n = math.min(#a, #b)
  for i = 1, n do
    local x, y = string.byte(a, i), string.byte(b, i)
    if x ~= y then return x < y end
  end
  return #a <= #b
end
local max = ARGV[1]
local exclusive = ARGV[2] == '1'
local range = max
if exclusive then range = '(' .. max end
local count = tonumber(ARGV[5])
if ARGV[3] == '' then
  return redis.call('ZRANGEBYSCORE', KEYS[1], '-inf', range, 'WITHSCORES', 'LIMIT', 0, count)
end
local out = {}
local rank = redis.call('ZRANK', KEYS[1], ARGV[3])
if rank then
  local limit = tonumber(max)
  local r = redis.call('ZRANGE', KEYS[1], rank + 1, rank + count, 'WITHSCORES')
  for i = 1, #r, 2 do
    local score = tonumber(r[i + 1])
    if score > limit or (exclusive and score == limit) then break end
    out[#out + 1] = r[i]
    out[#out + 1] = r[i + 1]
  end
  return out
end
local cursorScore = tonumber(ARGV[4])
local n, offset = 0, 0
while n < count do
  local r = redis.call('ZRANGEBYSCORE', KEYS[1], ARGV[4], range, 'WITHSCORES', 'LIMIT', offset, 128)
  if #r == 0 then break end
  for i = 1, #r, 2 do
    if not (tonumber(r[i + 1]) == cursorScore and le(r[i], ARGV[3])) then
      out[#out + 1] = r[i]
      out[#out + 1] = r[i + 1]
      n = n + 1
      if n == count then break end
    end
  end
  offset = offset + #r / 2
end
return out
`)

// page reads up to count members of key with score up to maxScore (below it when
// exclusive) after the cursor.
func (s *Store) page(ctx context.Context, key string, maxScore int64, exclusive bool, after member, count int) ([]member, error) {
	res, err := pageScript.Run(ctx, s.Client(), []string{key},
		maxScore, boolArg(exclusive), after.id, after.score, count).StringSlice()
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "read saga index page from Redis")
	}
	out := make([]member, 0, len(res)/2)
	for i := 0; i+1 < len(res); i += 2 {
		out = append(out, member{id: res[i], score: res[i+1]})
	}
	return out, nil
}

// FetchRecoverable returns up to limit non-terminal instances that are
// mid-compensation or past their deadline, restricted to definition when it is
// non-empty. A non-positive limit means no cap.
//
// The index is shared by every definition and also holds candidates whose
// lease is still active, so an entry counts toward limit only once it matches
// definition and is recoverable at now; otherwise non-matching entries could
// fill every batch and starve the rest. The due ids are read in pages of at
// least pageSize members after a cursor, so the transfer is bounded by the
// pages needed to fill limit, not by the backlog. An entry that cannot be
// decoded is skipped and logged by class so it cannot stop the scan.
func (s *Store) FetchRecoverable(ctx context.Context, definition string, now time.Time, limit int) ([]*saga.Instance, error) {
	count := pageSize
	if limit > 0 {
		count = max(count, limit)
	}

	var (
		out   []*saga.Instance
		stale []string
		after member
	)
	for {
		members, err := s.page(ctx, s.indexKey(), now.UnixMilli(), false, after, count)
		if err != nil {
			return nil, err
		}
		insts, gone, err := s.loadInstances(ctx, members)
		if err != nil {
			return nil, err
		}
		stale = append(stale, gone...)

		for _, inst := range insts {
			if inst == nil {
				continue
			}
			if !inst.Corrupt && ((definition != "" && inst.Definition != definition) || !inst.Recoverable(now)) {
				continue
			}
			out = append(out, inst)
			if limit > 0 && len(out) == limit {
				break
			}
		}
		if (limit > 0 && len(out) == limit) || len(members) < count {
			break
		}
		after = members[len(members)-1]
	}

	s.removeStale(ctx, s.indexKey(), stale)
	return out, nil
}

// removeStale drops index members whose instance hash is gone, rechecking each
// one atomically so a member re-created since the scan is kept.
func (s *Store) removeStale(ctx context.Context, indexKey string, ids []string) {
	for _, id := range ids {
		_ = removeStaleScript.Run(ctx, s.Client(), []string{s.instanceKey(id), indexKey}, id).Err() // Best-effort cleanup.
	}
}

// removeStaleScript removes an index member only while its instance hash is
// still absent.
//
// KEYS[1]=instance hash, KEYS[2]=ZSET. ARGV[1]=member(id).
var removeStaleScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 0 then redis.call('ZREM', KEYS[2], ARGV[1]) end
return 1
`)

// loadInstances reads and decodes the instances for members in one pipeline.
// The result is aligned with members; an entry is nil when its instance key no
// longer exists (its id is also returned in gone). A record that cannot be
// decoded is returned as a stub with Corrupt set and the stored version, which
// the orchestrator quarantines, and is logged by class.
func (s *Store) loadInstances(ctx context.Context, members []member) (insts []*saga.Instance, gone []string, err error) {
	if len(members) == 0 {
		return nil, nil, nil
	}

	pipe := s.Client().Pipeline()
	cmds := make([]*redis.SliceCmd, len(members))
	for i, m := range members {
		cmds[i] = pipe.HMGet(ctx, s.instanceKey(m.id), hashFieldData, hashFieldVersion)
	}
	// Exec returns the first command error; each command is inspected
	// individually below, so its aggregate error is ignored here.
	_, _ = pipe.Exec(ctx)

	insts = make([]*saga.Instance, len(members))
	for i, cmd := range cmds {
		vals, cErr := cmd.Result()
		if cErr != nil {
			return nil, nil, coreerrs.WrapOperation(cErr, "read saga instance from Redis")
		}
		raw, ok := vals[0].(string)
		if !ok {
			gone = append(gone, members[i].id) // Key gone since it was indexed.
			continue
		}
		inst, dErr := decodeInstance([]byte(raw))
		if dErr != nil {
			var version int64
			if v, ok := vals[1].(string); ok {
				version, _ = strconv.ParseInt(v, 10, 64)
			}
			s.opts.logger.WarnContext(ctx, "saga: undecodable instance record",
				slog.String("class", "undecodable instance record"), slog.Int("bytes", len(raw)))
			insts[i] = &saga.Instance{ID: members[i].id, Version: version, Corrupt: true}
			continue
		}
		insts[i] = inst
	}
	return insts, gone, nil
}

// Delete removes an instance and its index entries. Deleting a missing
// instance is not an error.
func (s *Store) Delete(ctx context.Context, id string) error {
	if err := checkID(id); err != nil {
		return err
	}
	if err := deleteScript.Run(ctx, s.Client(),
		[]string{s.instanceKey(id), s.indexKey(), s.terminalKey()}, id,
	).Err(); err != nil {
		return coreerrs.WrapOperation(err, "delete saga instance from Redis")
	}
	return nil
}

// DeleteTerminal removes up to limit terminal instances last updated before
// olderThan, restricted to definition when it is non-empty, and returns how
// many it removed. A non-positive limit means no cap. The terminal index holds
// every definition, so entries of other definitions are read and skipped. The
// last update is compared at millisecond granularity.
func (s *Store) DeleteTerminal(ctx context.Context, definition string, olderThan time.Time, limit int) (int, error) {
	count := pageSize
	if limit > 0 {
		count = max(count, limit)
	}

	var (
		deleted int
		stale   []string
		after   member // The last entry kept in the index: deleted ones leave it.
	)
	for {
		members, err := s.page(ctx, s.terminalKey(), olderThan.UnixMilli(), true, after, count)
		if err != nil {
			return deleted, err
		}
		insts, gone, err := s.loadInstances(ctx, members)
		if err != nil {
			return deleted, err
		}
		stale = append(stale, gone...)

		for i, inst := range insts {
			match := inst != nil && !inst.Corrupt && inst.Status.IsTerminal() && (definition == "" || inst.Definition == definition)
			if !match || (limit > 0 && deleted == limit) {
				after = members[i]
				continue
			}
			// The delete rechecks the cutoff, definition and version atomically, so
			// a record replaced since it was read is kept.
			n, err := deleteEligibleScript.Run(ctx, s.Client(),
				[]string{s.instanceKey(inst.ID), s.indexKey(), s.terminalKey()},
				inst.ID, inst.Version, olderThan.UnixMilli(), definition).Int64()
			if err != nil {
				return deleted, coreerrs.WrapOperation(err, "delete terminal saga instance from Redis")
			}
			if n == 0 {
				after = members[i]
				continue
			}
			deleted++
		}
		if (limit > 0 && deleted == limit) || len(members) < count {
			break
		}
	}
	s.removeStale(ctx, s.terminalKey(), stale)
	return deleted, nil
}

// instanceKey builds the prefixed hash key for an instance ID.
func (s *Store) instanceKey(id string) string { return s.Key(instanceKeyTag + id) }

// indexKey builds the prefixed key of the recoverable sorted set.
func (s *Store) indexKey() string { return s.Key(recoverableIndex) }

// terminalKey builds the prefixed key of the terminal sorted set.
func (s *Store) terminalKey() string { return s.Key(terminalIndex) }

// ttlMs renders the configured TTL as a millisecond string ("0" = persist).
func (s *Store) ttlMs() string {
	if s.opts.ttl <= 0 {
		return "0"
	}
	return strconv.FormatInt(s.opts.ttl.Milliseconds(), 10)
}

// trimBefore is the terminal-index score below which an entry belongs to a hash
// the TTL has already expired, so every terminal write also drops those entries
// and a TTL-only deployment's index stays bounded. 0 when no TTL is set.
func (s *Store) trimBefore() int64 {
	if s.opts.ttl <= 0 {
		return 0
	}
	return time.Now().Add(-s.opts.ttl).UnixMilli()
}

// recoverScore reports the recoverable-index score for an instance and whether
// it should be indexed at all. A COMPENSATING instance is always due (score 0);
// a RUNNING instance with a deadline is due at that deadline; everything else
// (terminal, or RUNNING without a deadline) is not indexed.
func recoverScore(inst *saga.Instance) (score float64, indexed bool) {
	switch {
	case inst.Status.IsTerminal():
		return 0, false
	case inst.LeaseOwner != "":
		// Truncated, never rounded up: the index is a coarse filter and Recoverable
		// decides exactly, so a lease expiring within the query's millisecond is
		// fetched and checked instead of hidden until the next one.
		return float64(inst.LeaseUntil.UnixMilli()), true
	case inst.Status == saga.StatusCompensating:
		return 0, true
	case inst.Status == saga.StatusRunning && !inst.Deadline.IsZero():
		return float64(inst.Deadline.UnixMilli()), true
	default:
		return 0, false
	}
}

// terminalArgs reports the "1"/"0" terminal flag and the terminal-index score
// (the last update in Unix milliseconds) of an instance.
func terminalArgs(inst *saga.Instance) (flag string, score int64) {
	if !inst.Status.IsTerminal() {
		return "0", 0
	}
	if inst.UpdatedAt.IsZero() {
		return "1", 0
	}
	return "1", inst.UpdatedAt.UnixMilli()
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
