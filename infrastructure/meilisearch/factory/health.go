// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"log/slog"

	"github.com/altessa-s/go-atlas/data/meilisearch"
	"github.com/altessa-s/go-atlas/observability/health"
)

var _ health.Checker = (*meilisearchHealthChecker)(nil)

// meilisearchHealthChecker implements [health.Checker] for a [meilisearch.Client].
type meilisearchHealthChecker struct {
	client *meilisearch.Client
	logger *slog.Logger
}

// CheckHealth implements [health.Checker].
func (c *meilisearchHealthChecker) CheckHealth(ctx context.Context) health.ServingStatus {
	if err := c.client.Health(ctx); err != nil {
		c.logger.Warn("meilisearch health check failed", slog.Any("error", err))
		return health.StatusNotServing
	}
	return health.StatusServing
}
