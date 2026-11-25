// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisutils

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"
)

// ErrNotStandalonePrimary indicates a missing or conflicting server topology response.
var ErrNotStandalonePrimary = errors.New("cannot verify a standalone Redis primary")

type topologyReader interface {
	*redis.Client | *redis.Conn
	Do(context.Context, ...any) *redis.Cmd
	Info(context.Context, ...string) *redis.StringCmd
}

// VerifyStandalonePrimary requires both a primary ROLE and disabled cluster mode.
func VerifyStandalonePrimary[T topologyReader](ctx context.Context, c T) error {
	if c == nil {
		return ErrNotStandalonePrimary
	}
	role, err := c.Do(ctx, "ROLE").Slice()
	if err != nil {
		return fmt.Errorf("%w: ROLE: %w", ErrNotStandalonePrimary, err)
	}
	if len(role) == 0 || role[0] != "master" {
		return ErrNotStandalonePrimary
	}
	info, err := c.Info(ctx, "cluster").Result()
	if err != nil {
		return fmt.Errorf("%w: INFO cluster: %w", ErrNotStandalonePrimary, err)
	}
	for line := range strings.Lines(info) {
		if strings.TrimRight(line, "\r\n") == "cluster_enabled:0" {
			return nil
		}
	}
	return ErrNotStandalonePrimary
}
