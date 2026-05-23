// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=options

import (
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas/core/retry"
	"github.com/altessa-s/go-atlas/observability/health"
	"github.com/altessa-s/go-atlas/observability/metrics"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

const (
	// DefaultOperationsTimeout is the default timeout for storage operations.
	DefaultOperationsTimeout = time.Second * 15
	// DefaultMaxRetries is the default maximum number of retry attempts.
	DefaultMaxRetries = 3
	// DefaultBaseDelay is the default initial delay between retries.
	DefaultBaseDelay = time.Second
	// DefaultMaxDelay is the default maximum delay between retries.
	DefaultMaxDelay = time.Minute
	// DefaultMultiplier is the default exponential backoff multiplier.
	DefaultMultiplier = 2.0
	// DefaultMaxCacheSize is the default maximum number of cached entries.
	DefaultMaxCacheSize = 1000
)

// WithCache sets a pre-initialized cache instance for the Manager.
// This is the only way to configure caching behavior - you create the cache
// with desired settings and pass it to the Manager.
//
// The cache must implement the Cache[string, *Value[T]] interface where T matches
// the Manager's type parameter. Type safety is enforced at runtime.
//
// Parameters:
//   - cache: A pre-initialized cache instance implementing Cache[string, *Value[T]]
//
// Usage examples:
//   - WithCache(NewStandardCache[string, *Value[string]](1000))
//   - WithCache(NewShardedCache[string, *Value[string]](5000, WithShardCount(32)))
//   - WithCache(NewShardedCache[string, *Value[string]](10000)) // auto shard count
//
// If no cache is provided, a default standard cache with size 1000 will be used.
//
// Returns an option function that sets the cache instance for the Manager.
func WithCache[T any](cache Cache[string, *Value[T]]) Option {
	return func(opts *options) {
		opts.cache = cache
	}
}

// options holds all configuration settings for the Manager.
type options struct {
	// logger for Manager operations (default: discard logger)
	logger *slog.Logger
	// maxRetries is the maximum number of retry attempts (default: 3)
	maxRetries int `optgen:"default=DefaultMaxRetries"`
	// exponentialConfig for exponential backoff retry strategy
	exponentialConfig retry.ExponentialConfig `optgen:"default=defaultExponentialConfig()"`
	// cache is a pre-initialized cache instance (optional)
	cache any `opt:"-"` // Will be type-asserted to Cache[string, *Value[T]] in Manager
	// negativeFilter is a probabilistic filter for negative caching (optional)
	negativeFilter Filter
	// healthCoordinator for auto-registration with health system (optional)
	healthCoordinator *health.Coordinator
	// scheduler is the scheduler for background task registration (optional)
	scheduler corescheduler.TaskRegistrar `optgen:"notnil"`
	// updateSchedule is the cron schedule for cache update task
	updateSchedule string `opt:"-"`
	// runOnStart triggers an immediate update cycle when starting
	runOnStart bool `opt:"-"`
	// collector for metrics collection
	collector metrics.Collector `optgen:"notnil"`
}

func defaultExponentialConfig() retry.ExponentialConfig {
	return retry.ExponentialConfig{
		BaseDelay: DefaultBaseDelay,
		MaxDelay:  DefaultMaxDelay,
		Factor:    DefaultMultiplier,
	}
}

// WithUpdateSchedule configures the update cycle task for scheduler.
// This task will be registered if scheduler is provided via WithScheduler.
func WithUpdateSchedule(schedule string, runOnStart bool) Option {
	return func(o *options) {
		o.updateSchedule = schedule
		o.runOnStart = runOnStart
	}
}
