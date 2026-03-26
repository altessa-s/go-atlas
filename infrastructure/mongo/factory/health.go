// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"log/slog"

	"go.mongodb.org/mongo-driver/v2/mongo/readpref"

	"github.com/altessa-s/go-atlas/observability/health"

	datamongo "github.com/altessa-s/go-atlas/data/mongo"
)

var _ health.Checker = (*mongoHealthChecker)(nil)

// mongoHealthChecker implements health.Checker for a Mongo wrapper.
type mongoHealthChecker struct {
	m      *datamongo.Mongo
	logger *slog.Logger
}

// CheckHealth implements health.Checker.
func (c *mongoHealthChecker) CheckHealth(ctx context.Context) health.ServingStatus {
	client := c.m.Client()
	if client == nil {
		c.logger.Warn("mongo health check failed: client is nil")
		return health.StatusNotServing
	}
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		c.logger.Warn("mongo health check failed", slog.Any("error", err))
		return health.StatusNotServing
	}
	return health.StatusServing
}
