// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas/data/probfilter"
	"github.com/altessa-s/go-atlas/observability/metrics"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// --- FilterBuilder dependency methods ---

// UseLogger sets the logger for the filter builder and all created components.
func (b *FilterBuilder) UseLogger(v *slog.Logger) *FilterBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *FilterBuilder) UseDefaultLogger() *FilterBuilder {
	return b.UseLogger(slog.Default())
}

// UseRedisClient sets the Redis client used for Redis-backed filter storages.
func (b *FilterBuilder) UseRedisClient(v redis.UniversalClient) *FilterBuilder {
	b.redisClient = v
	return b
}

// UseDataLoader sets the source a Bloom filter is rebuilt from. Without it the
// rebuildOnStart and rebuildCron settings are inert.
func (b *FilterBuilder) UseDataLoader(v probfilter.DataLoader) *FilterBuilder {
	b.loader = v
	return b
}

// UseScheduler sets the scheduler that runs periodic rebuilds of a Redis
// filter per rebuildCron (an in-memory filter uses a process-local cron). It
// takes effect only together with [FilterBuilder.UseDataLoader].
func (b *FilterBuilder) UseScheduler(v corescheduler.TaskRegistrar) *FilterBuilder {
	b.scheduler = v
	return b
}

// --- ManagerBuilder dependency methods ---

// UseLogger sets the logger for the manager builder and all created components.
func (b *ManagerBuilder) UseLogger(v *slog.Logger) *ManagerBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *ManagerBuilder) UseDefaultLogger() *ManagerBuilder {
	return b.UseLogger(slog.Default())
}

// UseRedisClient sets the Redis client used for Redis-backed filter storages.
func (b *ManagerBuilder) UseRedisClient(v redis.UniversalClient) *ManagerBuilder {
	b.redisClient = v
	return b
}

// UseDataLoader sets the source the filter named name is rebuilt from; see
// [FilterBuilder.UseDataLoader].
func (b *ManagerBuilder) UseDataLoader(name string, v probfilter.DataLoader) *ManagerBuilder {
	if b.loaders == nil {
		b.loaders = make(map[string]probfilter.DataLoader)
	}
	b.loaders[name] = v
	return b
}

// UseScheduler sets the scheduler that runs periodic filter rebuilds; see
// [FilterBuilder.UseScheduler].
func (b *ManagerBuilder) UseScheduler(v corescheduler.TaskRegistrar) *ManagerBuilder {
	b.scheduler = v
	return b
}

// UseCollector sets the metrics collector of the built [probfilter.Manager],
// enabling the probfilter metrics for every configured filter.
func (b *ManagerBuilder) UseCollector(v metrics.Collector) *ManagerBuilder {
	b.collector = v
	return b
}
