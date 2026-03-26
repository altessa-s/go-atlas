// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"log/slog"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/observability/health"
)

var _ health.Checker = (*redisHealthChecker)(nil)

// redisHealthChecker implements health.Checker for a Redis client.
type redisHealthChecker struct {
	client redis.UniversalClient
	logger *slog.Logger
}

// CheckHealth implements health.Checker.
func (c *redisHealthChecker) CheckHealth(ctx context.Context) health.ServingStatus {
	if err := c.client.Ping(ctx).Err(); err != nil {
		c.logger.Warn("redis health check failed", slog.Any("error", err))
		return health.StatusNotServing
	}
	return health.StatusServing
}
