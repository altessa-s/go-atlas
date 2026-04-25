// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisutils

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// GetBytes gets key from Redis and returns notFoundErr when the key doesn't exist.
func GetBytes(ctx context.Context, client redis.UniversalClient, key string, notFoundErr error) ([]byte, error) {
	b, err := client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, notFoundErr
		}
		return nil, err
	}
	return b, nil
}

// SetBytes stores value under key with ttl.
func SetBytes(ctx context.Context, client redis.UniversalClient, key string, value []byte, ttl time.Duration) error {
	return client.Set(ctx, key, value, ttl).Err()
}

// Del deletes key (idempotent).
func Del(ctx context.Context, client redis.UniversalClient, key string) error {
	return client.Del(ctx, key).Err()
}
