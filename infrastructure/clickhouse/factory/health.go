// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"log/slog"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/altessa-s/go-atlas/observability/health"
)

var _ health.Checker = (*clickHouseHealthChecker)(nil)

// clickHouseHealthChecker implements [health.Checker] for a ClickHouse
// connection. The driver connects lazily, so this is the first thing that
// actually reaches the server.
type clickHouseHealthChecker struct {
	conn   driver.Conn
	logger *slog.Logger
}

// CheckHealth implements [health.Checker].
func (c *clickHouseHealthChecker) CheckHealth(ctx context.Context) health.ServingStatus {
	if c.conn == nil {
		c.logger.Warn("clickhouse health check failed: connection is nil")

		return health.StatusNotServing
	}

	if err := c.conn.Ping(ctx); err != nil {
		c.logger.Warn("clickhouse health check failed", slog.Any("error", err))

		return health.StatusNotServing
	}

	return health.StatusServing
}
