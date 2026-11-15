// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides fluent builders for creating probabilistic filters
// and filter managers from configuration.
//
// Two builder types are available:
//
// [FilterBuilder] creates individual Bloom or Cuckoo filters:
//
//	filter, err := factory.NewFilter("my-filter", filterCfg, defaults).
//	    UseLogger(logger).
//	    UseBloomStorage(probfilterconfig.StorageTypeRedis, bloomredisfactory.Builder(redisClient)).
//	    Build(ctx)
//
// [ManagerBuilder] creates a [probfilter.Manager] with all configured filters:
//
//	mgr, err := factory.NewManager(cfg.ProbabilisticFilter).
//	    UseLogger(logger).
//	    UseBloomStorage(probfilterconfig.StorageTypeRedis, bloomredisfactory.Builder(redisClient)).
//	    UseCuckooStorage(probfilterconfig.StorageTypeRedis, cuckooredisfactory.Builder(redisClient)).
//	    Build(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer mgr.Close()
//
// # Storages
//
// The builders build only the in-memory storages; the Redis ones register
// from factory/cache/probfilter/bloom/storages/redis and
// factory/cache/probfilter/cuckoo/storages/redis through UseBloomStorage and
// UseCuckooStorage, and a configured type without one fails at Build naming
// the package to add.
//
// # Rebuilds
//
// Bloom rebuild settings need a data source: with [FilterBuilder.UseDataLoader]
// (or [ManagerBuilder.UseDataLoader]) rebuildOnStart rebuilds the filter
// inside Build, and rebuildCron rebuilds it periodically — with a
// process-local cron for an in-memory filter, and through the scheduler
// ([FilterBuilder.UseScheduler]) for a shared Redis filter. Without a loader
// both settings are inert. The builder only maps the settings onto the
// filter's rebuild options; the lifecycle itself is
// [bloom.Filter.StartRebuild]. A filter returned by Build after rebuildOnStart is
// populated: an initial rebuild refused because another process is
// rebuilding the shared filter fails Build, unless
// [FilterBuilder.TolerateRebuildInProgress] opts into an unpopulated filter.
//
// # Cuckoo settings
//
// capacityMultiplier maps to the RedisBloom EXPANSION argument and has no
// in-memory equivalent.
package factory
