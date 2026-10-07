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

// MinPeriod is the shortest budget period [New] accepts.
const MinPeriod = time.Second

// New creates a new budget [Limiter] allowing limit requests per key within
// each period, counted in storage. It returns [ErrInvalidLimit] when limit is
// not positive, [ErrInvalidPeriod] when period is shorter than [MinPeriod] and
// [ErrNilStorage] when storage is nil.
func New(limit int64, period time.Duration, storage storages.Storage, opts ...Option) (*Limiter, error) {
	switch {
	case limit < 1:
		return nil, fmt.Errorf("%w: %d", ErrInvalidLimit, limit)
	case period < MinPeriod:
		return nil, fmt.Errorf("%w: %s, minimum %s", ErrInvalidPeriod, period, MinPeriod)
	case storage == nil:
		return nil, ErrNilStorage
	}

	o := newOptions(opts...)

	return &Limiter{
		storage: storage,
		limit:   limit,
		period:  period,
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
