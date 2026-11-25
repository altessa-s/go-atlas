// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"log/slog"

	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/store/meilisearch"
)

var _ health.Checker = (*meilisearchHealthChecker)(nil)

// meilisearchHealthChecker implements [health.Checker] for a [meilisearch.Client].
type meilisearchHealthChecker struct {
	client meilisearch.HealthChecker
	logger *slog.Logger
}

// CheckHealth implements [health.Checker].
func (c *meilisearchHealthChecker) CheckHealth(ctx context.Context) health.ServingStatus {
	if err := c.client.Health(ctx); err != nil {
		c.logger.WarnContext(ctx, "meilisearch health check failed", slog.String("category", "health_check_failed"))
		return health.StatusNotServing
	}
	return health.StatusServing
}
