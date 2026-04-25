// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package health

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

import (
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas/observability/metrics"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

const (
	// DefaultWatcherChannelBuffer defines the buffer size for watcher notification channels.
	DefaultWatcherChannelBuffer = 10

	// DefaultMaxWatchersPerService defines the maximum number of concurrent watchers per service.
	DefaultMaxWatchersPerService = 1000

	// DefaultNumShards defines the number of shards for distributing lock contention.
	DefaultNumShards = 32

	// DefaultMaxConcurrentHealthChecks limits concurrent health checks in List method.
	DefaultMaxConcurrentHealthChecks = 10

	// DefaultStatusCacheTTL defines how long to cache health check results.
	DefaultStatusCacheTTL = 500 * time.Millisecond

	// DefaultCheckTimeout defines the maximum time to wait for a single health check in List method.
	DefaultCheckTimeout = 2 * time.Second

	// DefaultAdaptiveBufferThreshold defines when to increase channel buffer size.
	DefaultAdaptiveBufferThreshold = 100

	// DefaultAdaptiveBufferMultiplier defines how much to increase buffer under load.
	DefaultAdaptiveBufferMultiplier = 2

	// DefaultMaxAdaptiveBuffer defines the maximum channel buffer size under load.
	DefaultMaxAdaptiveBuffer = 100
)

type options struct {
	watcherChannelBuffer      int           `optgen:"default=DefaultWatcherChannelBuffer" optval:"positive"`
	maxWatchersPerService     int           `optgen:"default=DefaultMaxWatchersPerService" optval:"positive"`
	numShards                 int           `optgen:"default=DefaultNumShards" optval:"positive"`
	maxConcurrentHealthChecks int           `optgen:"default=DefaultMaxConcurrentHealthChecks"  optval:"positive"`
	statusCacheTTL            time.Duration `optgen:"default=DefaultStatusCacheTTL"`
	checkTimeout              time.Duration `optgen:"default=DefaultCheckTimeout"`
	adaptiveBufferThreshold   int32         `optgen:"default=DefaultAdaptiveBufferThreshold"  optval:"positive"`
	adaptiveBufferMultiplier  int           `optgen:"default=DefaultAdaptiveBufferMultiplier"  optval:"positive"`
	maxAdaptiveBuffer         int           `optgen:"default=DefaultMaxAdaptiveBuffer"  optval:"positive"`
	logger                    *slog.Logger

	// Scheduler configuration
	scheduler     corescheduler.TaskRegistrar `optgen:"notnil"`
	checkSchedule string

	collector metrics.Collector `optgen:"notnil"`
}
