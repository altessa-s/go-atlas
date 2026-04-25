// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets

import (
	"context"
	"time"

	"github.com/altessa-s/go-atlas/observability/health"
)

// Health check timeout constants
const (
	DefaultHealthCheckTimeout  = 5 * time.Second
	DefaultCacheStaleThreshold = 10 * time.Minute
)

// ProviderHealthChecker defines optional health check interface for providers.
type ProviderHealthChecker interface {
	// CheckConnection verifies the provider's connection to the underlying storage.
	CheckConnection(ctx context.Context) error
}

var _ health.Checker = (*Manager[any])(nil)

// CheckHealth implements health.Checker.
func (t *Manager[T]) CheckHealth(ctx context.Context) health.ServingStatus {
	// Check if provider implements health checking
	if healthChecker, ok := t.secretStorage.(ProviderHealthChecker); ok {
		if err := healthChecker.CheckConnection(ctx); err != nil {
			return health.StatusNotServing
		}
	}

	// Check cache staleness
	lastUpdate := t.LastUpdateTime()
	if !lastUpdate.IsZero() && time.Since(lastUpdate) > DefaultCacheStaleThreshold {
		return health.StatusDegraded
	}

	return health.StatusServing
}
