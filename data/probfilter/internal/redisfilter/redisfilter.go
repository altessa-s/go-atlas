// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisfilter

import (
	"context"
	"fmt"
	"iter"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// batchSize is the maximum number of command arguments accumulated per batch,
// keeping individual Redis commands reasonably sized.
const batchSize = 1000

// Commands describes the RedisBloom command verbs of one probabilistic filter
// type (Bloom BF.*, Cuckoo CF.*).
type Commands struct {
	// Label names the filter type inside wrapped error operations; for
	// example "Bloom" yields "add to Redis Bloom filter".
	Label string
	// Exists is the membership-check command, e.g. "BF.EXISTS".
	Exists string
	// Add is the single-item insert command, e.g. "BF.ADD".
	Add string
	// AddBatch is the multi-item insert command, e.g. "BF.MADD" or "CF.INSERT".
	AddBatch string
	// BatchTokens are literal tokens placed between the filter key and the
	// items of a batch command, e.g. "ITEMS" for CF.INSERT.
	BatchTokens []string
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
func New(client redis.UniversalClient, filterKey string, cmds Commands, reserveArgs ...any) *Core {
	batchHeader := make([]any, 0, 2+len(cmds.BatchTokens))
	batchHeader = append(batchHeader, cmds.AddBatch, filterKey)
	for _, token := range cmds.BatchTokens {
		batchHeader = append(batchHeader, token)
	}

	return &Core{
		client:      client,
		filterKey:   filterKey,
		cmds:        cmds,
		reserveArgs: reserveArgs,
		batchHeader: batchHeader,
		opExists:    "check existence in Redis " + cmds.Label + " filter",
		opAdd:       "add to Redis " + cmds.Label + " filter",
		opBatch:     "batch add to Redis " + cmds.Label + " filter",
		opCreate:    "create Redis " + cmds.Label + " filter",
		opDelete:    "delete Redis " + cmds.Label + " filter",
		opInfo:      "get Redis " + cmds.Label + " filter info",
	}
}

// FilterKey returns the fully prefixed Redis key of the filter.
func (c *Core) FilterKey() string {
	return c.filterKey
}

// MightExist checks if a value might exist in the filter.
func (c *Core) MightExist(ctx context.Context, value string) (bool, error) {
	result, err := c.client.Do(ctx, c.cmds.Exists, c.filterKey, value).Int()
	if err != nil {
		return false, coreerrs.WrapOperation(err, c.opExists)
	}
	return result == 1, nil
}

// Add inserts a value into the filter, creating the filter first when it
// does not exist yet.
func (c *Core) Add(ctx context.Context, value string) error {
	_, err := c.client.Do(ctx, c.cmds.Add, c.filterKey, value).Result()
	if err != nil {
		// Check if filter doesn't exist and create it
		if notExist(err) {
			if ensureErr := c.EnsureFilter(ctx); ensureErr != nil {
				return ensureErr
			}
			_, err = c.client.Do(ctx, c.cmds.Add, c.filterKey, value).Result()
		}
		if err != nil {
			return coreerrs.WrapOperation(err, c.opAdd)
		}
	}
	return nil
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
	_, err := c.client.Do(ctx, args...).Result()
	if err != nil {
		// Check if filter doesn't exist and create it
		if notExist(err) {
			if ensureErr := c.EnsureFilter(ctx); ensureErr != nil {
				return ensureErr
			}
			_, err = c.client.Do(ctx, args...).Result()
		}
		if err != nil {
			return coreerrs.WrapOperation(err, c.opBatch)
		}
	}
	return nil
}

// EnsureFilter creates the filter with the configured reserve arguments if
// it doesn't exist. An already-existing filter is not an error.
func (c *Core) EnsureFilter(ctx context.Context) error {
	args := append([]any{c.cmds.Reserve, c.filterKey}, c.reserveArgs...)
	err := c.client.Do(ctx, args...).Err()
	if err != nil && !strings.Contains(err.Error(), "exists") {
		return coreerrs.WrapOperation(err, c.opCreate)
	}
	return nil
}

// Reserve creates the filter with explicit capacity arguments, overriding
// the configured ones. Unlike [Core.EnsureFilter] any failure is an error.
func (c *Core) Reserve(ctx context.Context, reserveArgs ...any) error {
	args := append([]any{c.cmds.Reserve, c.filterKey}, reserveArgs...)
	if err := c.client.Do(ctx, args...).Err(); err != nil {
		return coreerrs.WrapOperation(err, c.opCreate)
	}
	return nil
}

// DeleteFilter removes the filter key entirely.
func (c *Core) DeleteFilter(ctx context.Context) error {
	if err := c.client.Del(ctx, c.filterKey).Err(); err != nil {
		return coreerrs.WrapOperation(err, c.opDelete)
	}
	return nil
}

// Info runs the info command and returns the raw reply. found is false when
// the filter does not exist yet; that is not an error.
func (c *Core) Info(ctx context.Context) (result any, found bool, err error) {
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

// notExist reports whether err indicates the filter key does not exist yet.
func notExist(err error) bool {
	return strings.Contains(err.Error(), "not exist")
}

// InfoFields iterates the alternating key/value pairs of a RedisBloom
// *.INFO reply. Replies of unexpected shape and non-string keys are skipped.
func InfoFields(result any) iter.Seq2[string, any] {
	return func(yield func(string, any) bool) {
		v, ok := result.([]any)
		if !ok {
			return
		}
		for i := 0; i < len(v)-1; i += 2 {
			key, ok := v[i].(string)
			if !ok {
				continue
			}
			if !yield(key, v[i+1]) {
				return
			}
		}
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
