// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/core/runtime/singleflight"
	"github.com/altessa-s/go-atlas/core/time/clock"
	"github.com/altessa-s/go-atlas/data/cache/generation"
	"github.com/altessa-s/go-atlas/data/internal/redisbase"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// ErrRegressed is returned by [Store.Current] when Redis holds a generation
// lower than one this replica already observed (a failover restored an older
// value, or the key was lost and reseeded lower). The read fails closed so
// the caller uses the source of truth; the store has already tried to heal
// the key above every generation it knows.
var ErrRegressed = errors.New("generation: stored generation regressed")

// serverNow is the Lua snippet computing the Redis server time in
// microseconds (the [generation.Seed] unit) into the local variable now.
const serverNow = `
local t = redis.call('TIME')
local now = tonumber(t[1]) * 1000000 + tonumber(t[2])
`

// currentScript returns the generation stored under KEYS[1], seeding a missing
// key from the Redis server clock so a lost key never restarts from zero and
// replica clock skew plays no role.
var currentScript = redis.NewScript(`
local v = redis.call('GET', KEYS[1])
if v then
  return tonumber(v)
end` + serverNow + `
redis.call('SET', KEYS[1], string.format('%.0f', now))
return now
`)

// bumpScript sets the generation under KEYS[1] to the largest of the stored
// value plus one, ARGV[1] (the highest generation the calling replica has
// observed) plus one, and the Redis server time, and returns it. The result
// therefore exceeds everything the caller has seen, even when Redis lost or
// rolled back the key.
var bumpScript = redis.NewScript(`
local stored = tonumber(redis.call('GET', KEYS[1]) or '0')
local seen = tonumber(ARGV[1])` + serverNow + `
local v = math.max(stored + 1, seen + 1, now)
redis.call('SET', KEYS[1], string.format('%.0f', v))
return v
`)

// Store is a Redis-backed [generation.Store] shared by every replica that uses
// the same Redis and key prefix. Current reads the key and reuses the value
// locally for at most the configured max-staleness; Bump writes a value above
// both the stored one and every generation this replica has observed. New
// namespaces are seeded from the Redis server clock. Requires Redis 5 or
// later (scripts that call TIME before writing).
//
// Staleness bound: a Current call that starts more than max-staleness after
// a Bump returned (on any replica) returns at least the bumped value. The
// replica that bumped sees its own bump at once.
//
// Failures: Current returns an error when Redis cannot be read and the local
// value is older than max-staleness; it never extends the bound. When a Bump
// fails, the namespace is marked pending on this replica: until a later Bump
// or Current publishes the increment to Redis, Current returns an error, so
// this replica bypasses the cache instead of serving the data the write
// replaced. Other replicas cannot learn of an unpublished bump; for them the
// bound degrades to the lifetime of their cached entries.
//
// Regression: when Redis returns a generation below one this replica
// observed after that value was confirmed (failover to an older replica of
// the data, a lost key reseeded lower), Current fails closed with
// [ErrRegressed] without refreshing its local value, and heals the key with a
// Bump above every generation it knows; later reads resume from the healed
// value. A read overlapped by a bump of this replica never installs its
// value; if it returned more than the bump published, it is handled the same
// way (that value may predate the write and Redis has lost it). Bump itself
// succeeds only with a value it published that exceeds every generation the
// replica observed by the time it completes.
// Residual window: a replica that never observed the higher
// generation cannot detect the regression and may serve entries cached under
// the restored generation until a healing bump reaches it (at most
// max-staleness after a replica that knew the higher generation refreshes)
// or until those entries expire. Keep a finite entry TTL.
//
// Generation keys have no TTL: they must outlive every entry cached under
// them. One key exists per namespace.
type Store struct {
	base         redisbase.Base
	clock        clock.Clock
	maxStaleness time.Duration
	group        *singleflight.Group[string, observation]

	mu    sync.Mutex
	local map[string]localGen
	// bumps counts Bump calls started on this replica; it orders a failed
	// bump against later successful ones.
	bumps uint64
}

// localGen is what this replica knows of a namespace.
type localGen struct {
	// gen is the generation reads use: never decreases.
	gen uint64
	// seen is the highest generation this replica observed or published
	// (at least gen); a bump publishes above it.
	seen uint64
	// seq counts the finished bumps of this replica (successful or failed);
	// inflight counts the bumps running now. A read overlapped by a bump (seq changed, or a bump
	// in flight when it started or completed) never installs its value.
	seq      uint64
	inflight int
	// checkedAt is when the newest Redis call confirming gen started: gen is
	// at least the stored value at that instant.
	checkedAt time.Time
	// confirmedAt is when a Redis call that returned gen completed: a read
	// starting later must return at least gen unless Redis regressed.
	confirmedAt time.Time
	// pendingSince is nonzero while a failed Bump has not reached Redis: the
	// value of Store.bumps when the latest failed call started. Only an
	// increment that started later covers the writes behind the failures.
	pendingSince uint64
}

// observation is a generation returned by Redis with the start and end of
// the call and the bump state of the replica when the call started.
type observation struct {
	gen       uint64
	started   time.Time
	completed time.Time
	// seqAtStart and bumpingAtStart snapshot localGen.seq and
	// localGen.inflight > 0 before the call was sent.
	seqAtStart     uint64
	bumpingAtStart bool
}

var _ generation.Store = (*Store)(nil)

// New returns a store over client. Panics if client is nil.
//
// Example:
//
//	store := redis.New(client, redis.WithKeyPrefix("myapp:generation:"))
func New(client redis.UniversalClient, opts ...Option) *Store {
	o := newOptions(opts...)
	return &Store{
		base:         redisbase.NewBase(client, o.keyPrefix),
		clock:        o.clock,
		maxStaleness: o.maxStaleness,
		group:        singleflight.New[string, observation](),
		local:        make(map[string]localGen),
	}
}

// Current implements [generation.Store].
func (s *Store) Current(ctx context.Context, namespace string) (uint64, error) {
	called := s.clock.Now()

	s.mu.Lock()
	lg, ok := s.local[namespace]
	s.mu.Unlock()

	if ok && lg.pendingSince != 0 {
		return s.Bump(ctx, namespace)
	}
	if ok && s.maxStaleness > 0 && called.Sub(lg.checkedAt) < s.maxStaleness {
		return lg.gen, nil
	}

	if s.maxStaleness > 0 {
		obs, err := s.group.Do(ctx, namespace, func(ctx context.Context) (observation, error) {
			return s.read(ctx, namespace)
		})
		switch {
		case err == nil && !obs.started.Before(called.Add(-s.maxStaleness)):
			return s.observeRead(ctx, namespace, obs)
		case err != nil && !errors.Is(err, singleflight.ErrOverloaded):
			return 0, err
		}
		// A shared read that started before this call's staleness window
		// cannot vouch for it, and an overloaded group admits no new read:
		// read directly below.
	}

	obs, err := s.read(ctx, namespace)
	if err != nil {
		return 0, err
	}
	return s.observeRead(ctx, namespace, obs)
}

// bumpAttempts bounds how often Bump re-publishes when a concurrent read
// raised this replica's highest observed generation above the value it just
// published.
const bumpAttempts = 3

// errBumpNotConverged reports a Bump that could not publish a generation
// above everything this replica observed within bumpAttempts.
var errBumpNotConverged = errors.New("generation: bump did not exceed the observed generations")

// Bump implements [generation.Store]. It succeeds only with a generation it
// published to Redis that exceeds every generation this replica had observed
// when the call completed; a read that raised the observed maximum during the
// call makes it publish again (bounded). While it runs, it marks the
// namespace as being bumped, so no overlapping read installs its value. On
// failure the namespace stays pending on this replica until an increment
// reaches Redis.
func (s *Store) Bump(ctx context.Context, namespace string) (uint64, error) {
	s.mu.Lock()
	s.bumps++
	seq := s.bumps
	lg := s.local[namespace]
	lg.inflight++
	s.local[namespace] = lg
	seen := lg.seen
	s.mu.Unlock()

	started := s.clock.Now()
	for range bumpAttempts {
		v, err := bumpScript.Run(ctx, s.base.Client(), []string{s.base.Key(namespace)}, strconv.FormatUint(seen, 10)).Int64()
		if err != nil {
			s.failBump(namespace, seq)
			return 0, coreerrs.Wrapf(err, "bump generation %q", namespace)
		}
		published := uint64(v)

		s.mu.Lock()
		lg := s.local[namespace]
		if lg.seen >= published {
			// A concurrent read observed a generation at least as high:
			// publish again above it.
			seen = lg.seen
			s.mu.Unlock()
			continue
		}
		lg.gen, lg.seen = published, published
		lg.seq++
		lg.inflight--
		lg.confirmedAt = s.clock.Now()
		if started.After(lg.checkedAt) {
			lg.checkedAt = started
		}
		if lg.pendingSince != 0 && lg.pendingSince < seq {
			lg.pendingSince = 0
		}
		s.local[namespace] = lg
		s.mu.Unlock()
		return published, nil
	}
	s.failBump(namespace, seq)
	return 0, coreerrs.Wrapf(errBumpNotConverged, "bump generation %q", namespace)
}

// failBump ends the failed bump seq of namespace: it is no longer in flight,
// it counts as a finished bump for the overlap rule (a read that spanned it
// never installs its value) and the namespace stays pending. The latest
// failed bump is kept: a success must start after every failure to cover the
// writes behind them.
func (s *Store) failBump(namespace string, seq uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lg := s.local[namespace]
	lg.inflight--
	lg.seq++
	lg.pendingSince = max(lg.pendingSince, seq)
	s.local[namespace] = lg
}

// read loads the generation of namespace from Redis, seeding a missing key.
func (s *Store) read(ctx context.Context, namespace string) (observation, error) {
	s.mu.Lock()
	lg := s.local[namespace]
	s.mu.Unlock()

	started := s.clock.Now()
	v, err := currentScript.Run(ctx, s.base.Client(), []string{s.base.Key(namespace)}).Int64()
	if err != nil {
		return observation{}, coreerrs.Wrapf(err, "read generation %q", namespace)
	}
	return observation{
		gen: uint64(v), started: started, completed: s.clock.Now(),
		seqAtStart: lg.seq, bumpingAtStart: lg.inflight > 0,
	}, nil
}

// observeRead merges a read into the local view and returns the current
// generation. Every read raises the observed maximum (seen), so later bumps
// exceed it.
//
// A read overlapped by a bump of this replica (a bump completed since it
// started, or one was in flight when it started or is now) may predate the
// write behind that bump, so it never installs its value: a value at or below
// the current generation is discarded; a higher one proves that Redis lost a
// generation the bump did not exceed, so the read fails closed with
// [ErrRegressed] and the key is healed with a Bump above seen.
//
// A read that no bump overlapped installs its value monotonically, unless it
// started strictly after the current generation was confirmed and returned
// less: Redis went back, so it fails closed and heals the same way.
func (s *Store) observeRead(ctx context.Context, namespace string, obs observation) (uint64, error) {
	s.mu.Lock()
	lg := s.local[namespace]
	lg.seen = max(lg.seen, obs.gen)
	overlapped := obs.bumpingAtStart || lg.inflight > 0 || lg.seq != obs.seqAtStart
	regressed := false
	switch {
	case overlapped:
		regressed = obs.gen > lg.gen
	case obs.gen < lg.gen:
		regressed = obs.started.After(lg.confirmedAt)
	default:
		// Without a regression both values are lower bounds of a stored
		// value that only grows, so the larger one is at least the stored
		// value at the later check time.
		lg.gen = obs.gen
		if obs.completed.After(lg.confirmedAt) {
			lg.confirmedAt = obs.completed
		}
		if obs.started.After(lg.checkedAt) {
			lg.checkedAt = obs.started
		}
	}
	s.local[namespace] = lg
	gen := lg.gen
	s.mu.Unlock()

	if regressed {
		_, _ = s.Bump(ctx, namespace) // heal: write above every observed generation
		return 0, coreerrs.Wrapf(ErrRegressed, "namespace %q", namespace)
	}
	return gen, nil
}
