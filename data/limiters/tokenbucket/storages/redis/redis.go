// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/core/runtime/panics"
	"github.com/altessa-s/go-atlas/core/text/strings"
	"github.com/altessa-s/go-atlas/data/internal/redisbase"
	"github.com/altessa-s/go-atlas/data/limiters/tokenbucket/storages"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Provider implements a Redis-based rate limiting provider using sliding window algorithm.
type Provider struct {
	redisbase.Base
}

// New creates a new Redis-based rate limiting provider.
func New(client redis.UniversalClient, opt ...Option) *Provider {
	opts := newOptions(opt...)

	return &Provider{
		Base: redisbase.NewBase(client, opts.keyPrefix),
	}
}

// Allow checks if a request should be allowed based on the rate limit using sliding window algorithm.
func (p *Provider) Allow(ctx context.Context, key string, limit int64, period time.Duration) (*storages.LimitInfo, error) {
	panics.MustNonNil(ctx, "context must not be nil")
	panics.Must(key != "", "key must not be empty")
	panics.Must(limit > 0, "limit must be greater than 0")
	panics.Must(period > 0, "period must be greater than 0")

	redisKey := p.Key(key)
	now := time.Now().UnixMilli()
	windowSeconds := int64(period.Seconds())

	result, err := luaScript.Run(ctx, p.Client(), []string{redisKey}, windowSeconds, limit, now).Result()
	if err != nil {
		return nil, coreerrs.Wrap(err, "redis rate limit script error")
	}

	values, ok := result.([]any)
	if !ok || len(values) != 4 {
		return nil, errors.New("unexpected redis script result format")
	}

	info := &storages.LimitInfo{
		Remaining: toInt64(values[1]),
		Reset:     toInt64(values[2]),
	}

	allowed := toInt64(values[3])
	if allowed <= 0 {
		return info, storages.ErrLimitExceeded
	}

	return info, nil
}

// Reset resets the rate limit for a specific key.
func (p *Provider) Reset(ctx context.Context, key string) error {
	panics.MustNonNil(ctx, "context must not be nil")
	panics.Must(key != "", "key must not be empty")

	return p.Client().Del(ctx, p.Key(key)).Err()
}

// toInt64 extracts an int64 from a Redis Lua script result value
// without using fmt.Sprintf + parse round-trip.
func toInt64(v any) int64 {
	switch val := v.(type) {
	case int64:
		return val
	case string:
		return strings.ToInt64(val)
	default:
		return 0
	}
}

var _ storages.Storage = (*Provider)(nil)
