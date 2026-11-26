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
	"slices"
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
	lookupKeyTag     = "id:"
	definitionsKey   = "definitions"
)

// tagEscaper keeps a definition name from closing or reopening its hash tag.
var tagEscaper = strings.NewReplacer("%", "%25", "{", "%7B", "}", "%7D")

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

// claimScript reserves an instance ID for a definition in the ID lookup key.
// Returns 1 when the ID was free or already belongs to the definition, or the
// name of the definition that owns it.
//
// KEYS[1]=lookup key. ARGV[1]=definition.
var claimScript = redis.NewScript(`
local cur = redis.call('GET', KEYS[1])
if not cur then redis.call('SET', KEYS[1], ARGV[1]) return 1 end
if cur == ARGV[1] then return 1 end
return cur
`)

// reassignScript moves an ID lookup from a definition to another one only while
// it still names the expected definition, so a stale lookup left by a failed
// create or delete never blocks the ID. Returns 1 when it moved, 0 otherwise.
//
// KEYS[1]=lookup key. ARGV: 1=expected definition, 2=new definition.
var reassignScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) ~= ARGV[1] then return 0 end
redis.call('SET', KEYS[1], ARGV[2])
return 1
`)

// unlinkScript deletes an ID lookup only while it names the given definition.
//
// KEYS[1]=lookup key. ARGV[1]=definition.
var unlinkScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then redis.call('DEL', KEYS[1]) end
return 1
`)

// rollbackScript removes an instance created by a Create that then lost its ID
// to another definition, only while it is still the version that Create wrote.
//
// KEYS[1]=instance hash, KEYS[2]=recoverable ZSET, KEYS[3]=terminal ZSET.
// ARGV: 1=member(id), 2=version.
var rollbackScript = redis.NewScript(`
local v = redis.call('HGET', KEYS[1], 'v')
if not v or tonumber(v) ~= tonumber(ARGV[2]) then return 0 end
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
// Every write is a script over the instance hash and both indexes of one
// definition, so those keys carry the definition name as a hash tag
// ("<prefix>{d:<definition>}:...") and share a slot on Redis Cluster; different
// definitions spread across slots. Storage calls address an instance by ID
// alone, so a plain string key per ID ("<prefix>id:<ID>", its own slot) names the
// definition that owns it, and a set ("<prefix>definitions") lists the
// definitions for calls that span all of them. Those two live outside the
// one-slot instance scripts; each is touched by single-key scripts or commands
// only. Create claims the ID there, writes the instance, then re-checks the
// claim and rolls back when another definition won it, so IDs stay unique across
// definitions. Delete and retention drop the claim before the instance.
type Store struct {
	redisbase.Base
	opts *options
}

var _ saga.Storage = (*Store)(nil)

// New creates a Store with the given Redis client, standalone or Redis
// Cluster. It panics if client is nil. The key prefix needs no hash tag: the
// store tags the keys of each definition itself. A prefix containing a brace
// fails with an error matching [coreerrs.ErrInvalidArgument], because it would
// take over the hash tag.
//
// Example:
//
//	store, err := redis.New(rdb, redis.WithKeyPrefix("saga:"))
func New(client redis.UniversalClient, opt ...Option) (*Store, error) {
	opts := newOptions(opt...)
	if strings.ContainsAny(opts.keyPrefix, "{}") {
		return nil, fmt.Errorf("%w: key prefix %q contains a brace, which would override the per-definition hash tag",
			coreerrs.ErrInvalidArgument, opts.keyPrefix)
	}
	return &Store{
		Base: redisbase.NewBase(client, opts.keyPrefix),
		opts: opts,
	}, nil
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

	if err = s.claimID(ctx, inst.Definition, inst.ID); err != nil {
		return err
	}
	if err = s.Client().SAdd(ctx, s.definitionsKey(), inst.Definition).Err(); err != nil {
		return coreerrs.WrapOperation(err, "register saga definition in Redis")
	}
	res, err := createScript.Run(ctx, s.Client(),
		[]string{s.instanceKey(inst.Definition, inst.ID), s.indexKey(inst.Definition), s.terminalKey(inst.Definition)},
		data, inst.Version, score, inst.ID, boolArg(indexed), s.ttlMs(), terminal, terminalScore, s.trimBefore(),
	).Int64()
	if err != nil {
		return coreerrs.WrapOperation(err, "create saga instance in Redis")
	}
	if res == 0 {
		return saga.ErrInstanceExists
	}
	// Another definition may have taken the claim over while the instance was
	// being written; re-asserting it also restores a lookup a cleanup removed.
	if err = s.claimID(ctx, inst.Definition, inst.ID); err != nil {
		if errors.Is(err, saga.ErrInstanceExists) {
			_ = rollbackScript.Run(ctx, s.Client(),
				[]string{s.instanceKey(inst.Definition, inst.ID), s.indexKey(inst.Definition), s.terminalKey(inst.Definition)},
				inst.ID, inst.Version).Err() // Best effort: the hash is unreachable without the claim.
		}
		return err
	}
	s.syncLookupExpiry(ctx, inst.ID, terminal)
	return nil
}

// claimID reserves id for definition in the ID lookup. It returns
// [saga.ErrInstanceExists] when another definition owns the ID, unless that
// definition's hash is gone, which is a lookup left by a failed create or
// delete and is taken over.
func (s *Store) claimID(ctx context.Context, definition, id string) error {
	owner, err := claimScript.Run(ctx, s.Client(), []string{s.lookupKey(id)}, definition).Result()
	if err != nil {
		return coreerrs.WrapOperation(err, "claim saga instance id in Redis")
	}
	other, ok := owner.(string)
	if !ok {
		return nil // 1: the ID is ours.
	}
	n, err := s.Client().Exists(ctx, s.instanceKey(other, id)).Result()
	if err != nil {
		return coreerrs.WrapOperation(err, "check saga instance id owner in Redis")
	}
	if n > 0 {
		return saga.ErrInstanceExists
	}
	moved, err := reassignScript.Run(ctx, s.Client(), []string{s.lookupKey(id)}, other, definition).Int64()
	if err != nil {
		return coreerrs.WrapOperation(err, "claim saga instance id in Redis")
	}
	if moved == 0 {
		return saga.ErrInstanceExists
	}
	return nil
}

// definitionOf resolves the definition that owns id, or
// [saga.ErrInstanceNotFound] when no lookup exists.
func (s *Store) definitionOf(ctx context.Context, id string) (string, error) {
	def, err := s.Client().Get(ctx, s.lookupKey(id)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", saga.ErrInstanceNotFound
		}
		return "", coreerrs.WrapOperation(err, "resolve saga instance definition in Redis")
	}
	return def, nil
}

// syncLookupExpiry keeps the TTL of the ID lookup in step with its instance: a
// terminal instance expires with its hash, a live one persists. It is best
// effort: a failure leaves a lookup that outlives an expired hash (it reads as
// not found and the next create of the ID takes it over) or one that expires
// early, which the next write of a live instance repairs.
func (s *Store) syncLookupExpiry(ctx context.Context, id, terminal string) {
	if s.opts.ttl <= 0 {
		return
	}
	var err error
	if terminal == "1" {
		err = s.Client().PExpire(ctx, s.lookupKey(id), s.opts.ttl).Err()
	} else {
		err = s.Client().Persist(ctx, s.lookupKey(id)).Err()
	}
	if err != nil {
		s.opts.logger.WarnContext(ctx, "saga: cannot sync instance id lookup expiry", slog.String("class", "lookup expiry"))
	}
}

// unlinkLookup drops the ID lookup. It runs before the instance is deleted, so a
// create of the same ID that follows the delete is never unlinked by it.
func (s *Store) unlinkLookup(ctx context.Context, definition, id string) error {
	if err := unlinkScript.Run(ctx, s.Client(), []string{s.lookupKey(id)}, definition).Err(); err != nil {
		return coreerrs.WrapOperation(err, "remove saga instance id lookup from Redis")
	}
	return nil
}

// Get loads an instance by ID, returning [saga.ErrInstanceNotFound] when
// absent.
func (s *Store) Get(ctx context.Context, id string) (*saga.Instance, error) {
	if err := checkID(id); err != nil {
		return nil, err
	}
	def, err := s.definitionOf(ctx, id)
	if err != nil {
		return nil, err
	}
	data, err := s.Client().HGet(ctx, s.instanceKey(def, id), hashFieldData).Bytes()
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

	res, err := s.runUpdate(ctx, inst.Definition, inst, data, newVersion, score, indexed, terminal, terminalScore)
	if err != nil {
		return err
	}
	if res == -1 {
		// Absent under the named definition. An instance of another definition
		// is a rename, which would move its keys across slots: reject it.
		owner, lerr := s.definitionOf(ctx, inst.ID)
		if lerr != nil {
			return lerr
		}
		if owner != inst.Definition {
			return fmt.Errorf("%w: a saga instance cannot change its definition (stored %q, got %q)",
				coreerrs.ErrInvalidArgument, owner, inst.Definition)
		}
		return saga.ErrInstanceNotFound
	}
	switch res {
	case 1:
		inst.Version = newVersion
		s.syncLookupExpiry(ctx, inst.ID, terminal)
		return nil
	case 0:
		return saga.ErrVersionConflict
	default: // -1
		return saga.ErrInstanceNotFound
	}
}

// runUpdate runs the version-checked write against the keys of definition.
func (s *Store) runUpdate(ctx context.Context, definition string, inst *saga.Instance, data []byte, newVersion int64,
	score float64, indexed bool, terminal string, terminalScore int64,
) (int64, error) {
	res, err := updateScript.Run(ctx, s.Client(),
		[]string{s.instanceKey(definition, inst.ID), s.indexKey(definition), s.terminalKey(definition)},
		inst.Version, data, newVersion, score, inst.ID, boolArg(indexed), s.ttlMs(), terminal, terminalScore, s.trimBefore(),
	).Int64()
	if err != nil {
		return 0, coreerrs.WrapOperation(err, "update saga instance in Redis")
	}
	return res, nil
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
// Each definition has its own index, which also holds candidates whose lease is
// still active, so an entry counts toward limit only once it is recoverable at
// now; otherwise such entries could fill every batch and starve the rest. With
// an empty definition the registered definitions are walked in name order, so a
// busy early one can use the whole limit. The due ids are read in pages of at
// least pageSize members after a cursor, so the transfer is bounded by the
// pages needed to fill limit, not by the backlog. An entry that cannot be
// decoded is skipped and logged by class so it cannot stop the scan.
func (s *Store) FetchRecoverable(ctx context.Context, definition string, now time.Time, limit int) ([]*saga.Instance, error) {
	defs, err := s.scanDefinitions(ctx, definition)
	if err != nil {
		return nil, err
	}
	var out []*saga.Instance
	for _, def := range defs {
		remaining := 0
		if limit > 0 {
			if remaining = limit - len(out); remaining <= 0 {
				break
			}
		}
		insts, err := s.fetchRecoverable(ctx, def, definition, now, remaining)
		if err != nil {
			return nil, err
		}
		out = append(out, insts...)
	}
	return out, nil
}

// scanDefinitions lists the definitions a call spans: the one given, or every
// registered one, sorted, when definition is empty.
func (s *Store) scanDefinitions(ctx context.Context, definition string) ([]string, error) {
	if definition != "" {
		return []string{definition}, nil
	}
	defs, err := s.Client().SMembers(ctx, s.definitionsKey()).Result()
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "list saga definitions in Redis")
	}
	slices.Sort(defs)
	return defs, nil
}

// fetchRecoverable is [Store.FetchRecoverable] over the index of one definition.
// filter is the caller's definition argument; instances are checked against it
// as before. A non-positive limit means no cap.
func (s *Store) fetchRecoverable(ctx context.Context, def, filter string, now time.Time, limit int) ([]*saga.Instance, error) {
	definition := filter
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
		members, err := s.page(ctx, s.indexKey(def), now.UnixMilli(), false, after, count)
		if err != nil {
			return nil, err
		}
		insts, gone, err := s.loadInstances(ctx, def, members)
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

	s.removeStale(ctx, def, s.indexKey(def), stale)
	return out, nil
}

// removeStale drops index members whose instance hash is gone, rechecking each
// one atomically so a member re-created since the scan is kept.
func (s *Store) removeStale(ctx context.Context, def, indexKey string, ids []string) {
	for _, id := range ids {
		_ = removeStaleScript.Run(ctx, s.Client(), []string{s.instanceKey(def, id), indexKey}, id).Err() // Best-effort cleanup.
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
func (s *Store) loadInstances(ctx context.Context, def string, members []member) (insts []*saga.Instance, gone []string, err error) {
	if len(members) == 0 {
		return nil, nil, nil
	}

	pipe := s.Client().Pipeline()
	cmds := make([]*redis.SliceCmd, len(members))
	for i, m := range members {
		cmds[i] = pipe.HMGet(ctx, s.instanceKey(def, m.id), hashFieldData, hashFieldVersion)
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
	def, err := s.definitionOf(ctx, id)
	if err != nil {
		if errors.Is(err, saga.ErrInstanceNotFound) {
			return nil
		}
		return err
	}
	if err := s.unlinkLookup(ctx, def, id); err != nil {
		return err
	}
	if err := deleteScript.Run(ctx, s.Client(),
		[]string{s.instanceKey(def, id), s.indexKey(def), s.terminalKey(def)}, id,
	).Err(); err != nil {
		return coreerrs.WrapOperation(err, "delete saga instance from Redis")
	}
	return nil
}

// DeleteTerminal removes up to limit terminal instances last updated before
// olderThan, restricted to definition when it is non-empty, and returns how
// many it removed. A non-positive limit means no cap. Each definition has
// its own terminal index; with an empty definition they are walked in name
// order and limit spans them. The last update is compared at millisecond granularity.
func (s *Store) DeleteTerminal(ctx context.Context, definition string, olderThan time.Time, limit int) (int, error) {
	defs, err := s.scanDefinitions(ctx, definition)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, def := range defs {
		remaining := 0
		if limit > 0 {
			if remaining = limit - total; remaining <= 0 {
				break
			}
		}
		n, err := s.deleteTerminal(ctx, def, olderThan, remaining)
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// deleteTerminal is [Store.DeleteTerminal] over the terminal index of one
// definition. A non-positive limit means no cap.
func (s *Store) deleteTerminal(ctx context.Context, definition string, olderThan time.Time, limit int) (int, error) {
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
		members, err := s.page(ctx, s.terminalKey(definition), olderThan.UnixMilli(), true, after, count)
		if err != nil {
			return deleted, err
		}
		insts, gone, err := s.loadInstances(ctx, definition, members)
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
			ok, err := s.deleteEligible(ctx, definition, inst, olderThan)
			if err != nil {
				return deleted, err
			}
			if !ok {
				after = members[i]
				continue
			}
			deleted++
		}
		if (limit > 0 && deleted == limit) || len(members) < count {
			break
		}
	}
	s.removeStale(ctx, definition, s.terminalKey(definition), stale)
	return deleted, nil
}

// deleteEligible removes one terminal instance for retention and reports whether
// it did. The lookup goes first so a create of the ID after the delete is not
// unlinked by it. The delete rechecks the cutoff, definition and version
// atomically, so a record replaced since it was read is kept, and its lookup is
// restored.
func (s *Store) deleteEligible(ctx context.Context, definition string, inst *saga.Instance, olderThan time.Time) (bool, error) {
	if err := s.unlinkLookup(ctx, definition, inst.ID); err != nil {
		return false, err
	}
	n, err := deleteEligibleScript.Run(ctx, s.Client(),
		[]string{s.instanceKey(definition, inst.ID), s.indexKey(definition), s.terminalKey(definition)},
		inst.ID, inst.Version, olderThan.UnixMilli(), definition).Int64()
	if err != nil {
		return false, coreerrs.WrapOperation(err, "delete terminal saga instance from Redis")
	}
	if n == 0 {
		if err := s.claimID(ctx, definition, inst.ID); err != nil && !errors.Is(err, saga.ErrInstanceExists) {
			return false, err
		}
		return false, nil
	}
	return true, nil
}

// defKey builds a key of one definition. The definition name is the hash tag,
// so every key built for it hashes to one slot. The "d:" lead keeps the tag
// non-empty for an unnamed definition; braces in the name are escaped.
func (s *Store) defKey(definition, suffix string) string {
	return s.Key("{d:" + tagEscaper.Replace(definition) + "}:" + suffix)
}

// instanceKey builds the prefixed hash key for an instance of a definition.
func (s *Store) instanceKey(definition, id string) string {
	return s.defKey(definition, instanceKeyTag+id)
}

// indexKey builds the prefixed key of the recoverable sorted set of a definition.
func (s *Store) indexKey(definition string) string { return s.defKey(definition, recoverableIndex) }

// terminalKey builds the prefixed key of the terminal sorted set of a definition.
func (s *Store) terminalKey(definition string) string { return s.defKey(definition, terminalIndex) }

// lookupKey builds the prefixed key naming the definition that owns an ID.
func (s *Store) lookupKey(id string) string { return s.Key(lookupKeyTag + id) }

// definitionsKey builds the prefixed key of the set of known definitions.
func (s *Store) definitionsKey() string { return s.Key(definitionsKey) }

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
