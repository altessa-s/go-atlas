// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"log/slog"

	"github.com/nats-io/nats.go"

	"github.com/altessa-s/go-atlas/observability/health"
)

var _ health.Checker = (*natsHealthChecker)(nil)

// natsHealthChecker implements health.Checker for a NATS connection.
type natsHealthChecker struct {
	conn   *nats.Conn
	logger *slog.Logger
}

// CheckHealth implements health.Checker.
// Context is intentionally unused — IsConnected/IsReconnecting are non-blocking state reads.
func (c *natsHealthChecker) CheckHealth(_ context.Context) health.ServingStatus {
	if c.conn.IsConnected() {
		return health.StatusServing
	}
	if c.conn.IsReconnecting() {
		c.logger.Warn("nats health check: connection is reconnecting")
		return health.StatusDegraded
	}
	c.logger.Warn("nats health check: connection is not active")
	return health.StatusNotServing
}
