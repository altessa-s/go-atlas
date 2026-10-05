// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisfilter

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// batchSize is the maximum number of command arguments accumulated per batch,
// keeping individual Redis commands reasonably sized.
const batchSize = 1000

// ErrItemRejected is returned when a batch command reports a per-item
// failure (an error element, or CF.INSERT's -1 for a full filter) although the
// command as a whole succeeded.
var ErrItemRejected = errors.New("item rejected by Redis filter")

// Commands describes the RedisBloom command verbs of one probabilistic filter
// type (Bloom BF.*, Cuckoo CF.*).
type Commands struct {
	// Label names the filter type inside wrapped error operations; for
	// example "Bloom" yields "add to Redis Bloom filter".
	Label string
	// Exists is the membership-check command, e.g. "BF.EXISTS".
	Exists string
	// Add is the single-item insert command, e.g. "BF.INSERT". It must not
	// create a missing filter (see [Core.RebuildCommitted]); AddTokens are
	// literal tokens placed between the filter key and the item, e.g.
	// "NOCREATE", "ITEMS".
	Add       string
	AddTokens []string
	// AddBatch is the multi-item insert command, e.g. "BF.INSERT". Like Add
	// it must not create a missing filter; a rebuild's staging filter uses it
	// too, so a vanished staging key fails the rebuild.
	AddBatch string
	// BatchTokens are literal tokens placed between the filter key and the
	// items of a batch command, e.g. "NOCREATE", "ITEMS".
	BatchTokens []string
	// FalseRejects makes a false item reply of an insert a rejected item.
	// CF.INSERT answers a full filter with -1 under RESP2 but false under
	// RESP3; BF.INSERT's false only means "already present".
	FalseRejects bool
	// Reserve is the filter-creation command, e.g. "BF.RESERVE".
	Reserve string
	// Info is the statistics command, e.g. "BF.INFO".
	Info string
}

// Core implements the shared command execution, error wrapping, and
// ensure-filter plumbing of Redis-backed probabilistic filter storages.
// It is safe for concurrent use.
type Core struct {
	client      redis.UniversalClient
	filterKey   string
	cmds        Commands
	reserveArgs []any
	batchHeader []any
	// autoCreate makes Add and AddBatch reserve a missing filter. Staging
	// cores disable it so an expired staging key fails the rebuild instead of
	// being silently recreated empty.
	autoCreate bool
	// reserved records that the live filter was reserved with reserveArgs,
	// so the server never creates it implicitly with default parameters.
	reserved atomic.Bool
	// afterBatch, when set, runs after every successful batch command.
	afterBatch func(ctx context.Context) error
	// genKey holds the token of the live filter generation.
	genKey string
	// readyKey exists while the live key holds the contents a committed
	// rebuild renamed into place; see [Core.RebuildCommitted].
	readyKey string
	// deletesKey (hash) and deadlinesKey (sorted set) record delete requests
	// for at-most-once execution; see deleteScript.
	deletesKey   string
	deadlinesKey string
	// stagingsKey (hash) and stagingDeadlinesKey (sorted set) register created
	// staging ids so a staging key is created at most once; see stageScript.
	stagingsKey string
	// seqKey issues rebuild tickets, leaseKey holds the ticket of the rebuild
	// lease, committedKey the ticket of the last publishing rebuild; see
	// [Core.BeginRebuild] and commitScript.
	seqKey       string
	leaseKey     string
	committedKey string
	// leaseTTL is LeaseTTL; a field so tests can shorten it.
	leaseTTL            time.Duration
	stagingDeadlinesKey string
	// keyErr is ErrReservedKey for a filter key in the reserved namespace.
	keyErr error

	opExists string
	opAdd    string
	opBatch  string
	opCreate string
	opDelete string
	opInfo   string
}

// New creates a Core executing cmds against the fully prefixed filterKey.
// reserveArgs are the capacity arguments appended to the reserve command by
// [Core.EnsureFilter].
//
// A filterKey in the reserved [MetaPrefix] namespace is rejected: every
// operation of the returned Core fails with [ErrReservedKey].
func New(client redis.UniversalClient, filterKey string, cmds Commands, reserveArgs ...any) *Core {
	c := newCore(client, filterKey, cmds, reserveArgs...)
	if strings.HasPrefix(filterKey, MetaPrefix) {
		c.keyErr = ErrReservedKey
	}
	return c
}

// newCore builds a Core without the reserved-namespace check, for staging
// filters that live in that namespace.
func newCore(client redis.UniversalClient, filterKey string, cmds Commands, reserveArgs ...any) *Core {
	batchHeader := make([]any, 0, 2+len(cmds.BatchTokens))
	batchHeader = append(batchHeader, cmds.AddBatch, filterKey)
	for _, token := range cmds.BatchTokens {
		batchHeader = append(batchHeader, token)
	}

	return &Core{
		client:              client,
		filterKey:           filterKey,
		cmds:                cmds,
		reserveArgs:         reserveArgs,
		batchHeader:         batchHeader,
		autoCreate:          true,
		genKey:              metaKey(filterKey, "generation", ""),
		readyKey:            metaKey(filterKey, "ready", ""),
		deletesKey:          metaKey(filterKey, "deletes", ""),
		deadlinesKey:        metaKey(filterKey, "delete-deadlines", ""),
		stagingsKey:         metaKey(filterKey, "stagings", ""),
		stagingDeadlinesKey: metaKey(filterKey, "staging-deadlines", ""),
		seqKey:              metaKey(filterKey, "rebuild-seq", ""),
		leaseKey:            metaKey(filterKey, "rebuild-lease", ""),
		committedKey:        metaKey(filterKey, "rebuild-committed", ""),
		leaseTTL:            LeaseTTL,
		opExists:            "check existence in Redis " + cmds.Label + " filter",
		opAdd:               "add to Redis " + cmds.Label + " filter",
		opBatch:             "batch add to Redis " + cmds.Label + " filter",
		opCreate:            "create Redis " + cmds.Label + " filter",
		opDelete:            "delete Redis " + cmds.Label + " filter",
		opInfo:              "get Redis " + cmds.Label + " filter info",
	}
}

// MightExist checks if a value might exist in the filter.
func (c *Core) MightExist(ctx context.Context, value string) (bool, error) {
	if c.keyErr != nil {
		return false, c.keyErr
	}
	reply, err := c.client.Do(ctx, c.cmds.Exists, c.filterKey, value).Result()
	if err != nil {
		return false, coreerrs.WrapOperation(err, c.opExists)
	}
	exists, err := ToBool(reply)
	if err != nil {
		return false, coreerrs.WrapOperation(err, c.opExists)
	}
	return exists, nil
}

// Add inserts a value into the filter, creating the filter first when it
// does not exist yet. The insert command itself never creates the filter, so
// a missing filter is always recreated by [Core.EnsureFilter].
func (c *Core) Add(ctx context.Context, value string) error {
	args := make([]any, 0, 3+len(c.cmds.AddTokens))
	args = append(args, c.cmds.Add, c.filterKey)
	for _, token := range c.cmds.AddTokens {
		args = append(args, token)
	}
	args = append(args, value)
	return c.insert(ctx, args, c.opAdd)
}

// AddBatch inserts multiple values into the filter, chunking them into
// batch commands to avoid huge Redis commands.
func (c *Core) AddBatch(ctx context.Context, values iter.Seq[string]) error {
	currentBatch := make([]any, 0, batchSize+len(c.batchHeader))
	for v := range values {
		currentBatch = coreslices.AppendIf(currentBatch, len(currentBatch) == 0, c.batchHeader...)
		currentBatch = append(currentBatch, v)

		if len(currentBatch) >= batchSize {
			if err := c.sendBatch(ctx, currentBatch); err != nil {
				return err
			}
			// Reuse underlying array
			currentBatch = currentBatch[:0]
		}
	}

	if len(currentBatch) > 0 {
		return c.sendBatch(ctx, currentBatch)
	}

	return nil
}

func (c *Core) sendBatch(ctx context.Context, args []any) error {
	if err := c.insert(ctx, args, c.opBatch); err != nil {
		return err
	}
	if c.afterBatch != nil {
		return c.afterBatch(ctx)
	}
	return nil
}

// insert runs the insert command args: it reserves the filter before the
// first write, recreates a missing filter and retries once (unless
// autoCreate is off), and fails on a per-item rejection; errors are wrapped
// with op.
func (c *Core) insert(ctx context.Context, args []any, op string) error {
	if err := c.reserveOnce(ctx); err != nil {
		return err
	}
	reply, err := c.client.Do(ctx, args...).Result()
	if err != nil {
		// Check if filter doesn't exist and create it
		if c.autoCreate && notExist(err) {
			if ensureErr := c.EnsureFilter(ctx); ensureErr != nil {
				return ensureErr
			}
			reply, err = c.client.Do(ctx, args...).Result()
		}
		if err != nil {
			return coreerrs.WrapOperation(err, op)
		}
	}
	if err := checkBatchReply(reply, c.cmds.FalseRejects); err != nil {
		return coreerrs.WrapOperation(err, op)
	}
	return nil
}

// reserveOnce reserves the live filter with the configured arguments before
// its first write, so the first write does not have to fail on a missing
// filter first. An existing filter is kept. A failed attempt is retried on
// the next write.
func (c *Core) reserveOnce(ctx context.Context) error {
	if c.keyErr != nil {
		return c.keyErr
	}
	if !c.autoCreate || c.reserved.Load() {
		return nil
	}
	if err := c.EnsureFilter(ctx); err != nil {
		return err
	}
	c.reserved.Store(true)
	return nil
}

// checkBatchReply fails when a batch reply reports a per-item failure: an
// error element, a negative integer, or — with falseRejects — a false
// boolean (RESP3). Non-negative integers and other booleans are successes.
func checkBatchReply(reply any, falseRejects bool) error {
	items, ok := reply.([]any)
	if !ok {
		return nil
	}
	for i, item := range items {
		switch v := item.(type) {
		case error:
			return fmt.Errorf("%w: item %d: %w", ErrItemRejected, i, v)
		case int64:
			if v < 0 {
				return fmt.Errorf("%w: item %d: reply %d", ErrItemRejected, i, v)
			}
		case bool:
			if !v && falseRejects {
				return fmt.Errorf("%w: item %d: reply false", ErrItemRejected, i)
			}
		}
	}
	return nil
}

// reserveScript creates the filter with the reserve command. Creating a
// missing filter first deletes the ready marker in the same atomic step: a
// recreated filter does not hold the contents of the rebuild that set it
// (see [Core.RebuildCommitted]). Reserving an existing filter fails as the
// reserve command does and keeps the marker.
// KEYS[1] filter key; KEYS[2] ready marker; ARGV[1] reserve command;
// ARGV[2:] reserve args.
var reserveScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 0 then
  redis.call('DEL', KEYS[2])
end
local args = {ARGV[1], KEYS[1]}
for i = 2, #ARGV do args[#args + 1] = ARGV[i] end
return redis.call(unpack(args))
`)

// EnsureFilter creates the filter with the configured reserve arguments if
// it doesn't exist. An already-existing filter is not an error.
func (c *Core) EnsureFilter(ctx context.Context) error {
	if c.keyErr != nil {
		return c.keyErr
	}
	argv := append([]any{c.cmds.Reserve}, c.reserveArgs...)
	err := reserveScript.Run(ctx, c.client, []string{c.filterKey, c.readyKey}, argv...).Err()
	if err != nil && !strings.Contains(err.Error(), "exists") {
		return coreerrs.WrapOperation(err, c.opCreate)
	}
	return nil
}

// rebuildCommittedScript reports whether the ready marker and the filter key
// both exist. KEYS[1] ready marker; KEYS[2] filter key.
var rebuildCommittedScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 1 and redis.call('EXISTS', KEYS[2]) == 1 then
  return 1
end
return 0
`)

// RebuildCommitted reports whether the live filter holds the contents a
// committed rebuild renamed into place — by this or any other process —
// plus any values added since. The commit script writes a ready marker only
// after its rename, and every path of this package that creates the filter
// key deletes the marker atomically with the creation (the insert commands
// never create it), so the marker exists only while the live key is the one
// the last committed rebuild published. A filter key deleted or evicted
// since reports false. A filter key recreated by another client that does
// not delete the marker (for example a plain BF.RESERVE) is not detected:
// the filter key and its metadata keys must be deleted together.
func (c *Core) RebuildCommitted(ctx context.Context) (bool, error) {
	if c.keyErr != nil {
		return false, c.keyErr
	}
	n, err := rebuildCommittedScript.Run(ctx, c.client, []string{c.readyKey, c.filterKey}).Int()
	if err != nil {
		return false, coreerrs.WrapOperation(err, "check Redis "+c.cmds.Label+" filter rebuild state")
	}
	return n == 1, nil
}

// DeleteFilter removes the filter key entirely.
func (c *Core) DeleteFilter(ctx context.Context) error {
	if c.keyErr != nil {
		return c.keyErr
	}
	if err := c.client.Del(ctx, c.filterKey).Err(); err != nil {
		return coreerrs.WrapOperation(err, c.opDelete)
	}
	return nil
}

// Info runs the info command and returns the raw reply. found is false when
// the filter does not exist yet; that is not an error.
func (c *Core) Info(ctx context.Context) (result any, found bool, err error) {
	if c.keyErr != nil {
		return nil, false, c.keyErr
	}
	result, err = c.client.Do(ctx, c.cmds.Info, c.filterKey).Result()
	if err != nil {
		// Filter might not exist yet
		if notExist(err) {
			return nil, false, nil
		}
		return nil, false, coreerrs.WrapOperation(err, c.opInfo)
	}
	return result, true, nil
}

// notExist reports whether err indicates the filter key does not exist yet:
// "not exist", or "not found" as RedisBloom answers a NOCREATE insert or an
// INFO of a missing filter.
func notExist(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "not exist") || strings.Contains(msg, "not found")
}

// InfoFields iterates the key/value pairs of a RedisBloom *.INFO reply: the
// alternating array of RESP2 or the map of RESP3. Replies of unexpected shape
// and non-string keys are skipped; map order is unspecified.
func InfoFields(result any) iter.Seq2[string, any] {
	return func(yield func(string, any) bool) {
		switch v := result.(type) {
		case []any:
			for i := 0; i < len(v)-1; i += 2 {
				key, ok := v[i].(string)
				if !ok {
					continue
				}
				if !yield(key, v[i+1]) {
					return
				}
			}
		case map[any]any:
			for k, val := range v {
				key, ok := k.(string)
				if !ok {
					continue
				}
				if !yield(key, val) {
					return
				}
			}
		case map[string]any:
			for key, val := range v {
				if !yield(key, val) {
					return
				}
			}
		}
	}
}

// ToBool converts a RedisBloom boolean reply to bool: an integer 0/1 under
// RESP2, a boolean under RESP3.
func ToBool(v any) (bool, error) {
	switch val := v.(type) {
	case bool:
		return val, nil
	case int64:
		return val != 0, nil
	case int:
		return val != 0, nil
	case string:
		n, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return false, err
		}
		return n != 0, nil
	default:
		return false, fmt.Errorf("cannot convert %T to bool", v)
	}
}

// ToInt64 converts a RedisBloom info reply value to int64.
func ToInt64(v any) (int64, error) {
	switch val := v.(type) {
	case int64:
		return val, nil
	case int:
		return int64(val), nil
	case string:
		return strconv.ParseInt(val, 10, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to int64", v)
	}
}
