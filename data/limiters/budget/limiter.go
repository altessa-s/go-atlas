// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package budget

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/altessa-s/go-atlas/data/limiters/storages"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Limiter enforces a distributed request budget using a shared storage backend.
// It tracks the total number of requests per key within a configurable time period.
type Limiter struct {
	storage storages.Storage
	limit   int64
	period  time.Duration
	options *options
	metrics *limiterMetrics
}

// New creates a new budget [Limiter] with the given settings and storage backend.
// Returns an error if the settings are invalid or the storage is nil.
func New(cfg *Settings, storage storages.Storage, opts ...Option) (*Limiter, error) {
	if cfg == nil {
		return nil, fmt.Errorf("settings are required")
	}

	if err := cfg.Validate(); err != nil {
		return nil, coreerrs.Wrap(err, "invalid settings")
	}

	if storage == nil {
		return nil, fmt.Errorf("storage is required")
	}

	o := newOptions(opts...)

	return &Limiter{
		storage: storage,
		limit:   cfg.Limit,
		period:  cfg.Period,
		options: o,
		metrics: newLimiterMetrics(o.collector),
	}, nil
}

// Allow checks if a request identified by key is within the budget.
// Returns [ErrBudgetExhausted] when the budget for the period is exceeded.
// Returns storage errors as-is for fail-closed behavior.
func (l *Limiter) Allow(ctx context.Context, key string) error {
	_, err := l.storage.Allow(ctx, key, l.limit, l.period)
	if err != nil {
		if errors.Is(err, storages.ErrLimitExceeded) {
			l.metrics.requestsRejected.Inc()

			return ErrBudgetExhausted
		}

		l.metrics.limitCheckErrors.Inc()

		return err
	}

	l.metrics.requestsAllowed.Inc()

	return nil
}
