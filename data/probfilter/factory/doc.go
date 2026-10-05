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
//	    UseRedisClient(redisClient).
//	    Build()
//
// [ManagerBuilder] creates a [probfilter.Manager] with all configured filters:
//
//	mgr, err := factory.NewManager(cfg.ProbabilisticFilter).
//	    UseLogger(logger).
//	    UseRedisClient(redisClient).
//	    Build()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer mgr.Close()
//
// # Rebuilds
//
// Bloom rebuild settings need a data source: with [FilterBuilder.UseDataLoader]
// (or [ManagerBuilder.UseDataLoader]) rebuildOnStart rebuilds the filter
// inside Build, and rebuildCron rebuilds it periodically — with a
// process-local cron for an in-memory filter, and through the scheduler
// ([FilterBuilder.UseScheduler]) for a shared Redis filter. Without a loader
// both settings are inert. A filter returned by Build after rebuildOnStart is
// populated: an initial rebuild refused because another process is
// rebuilding the shared filter fails Build, unless
// [FilterBuilder.TolerateRebuildInProgress] opts into an unpopulated filter.
//
// # Cuckoo settings
//
// capacityMultiplier maps to the RedisBloom EXPANSION argument and has no
// in-memory equivalent; fingerprintSize and maxCapacity are deprecated and
// ignored, since no backend supports them.
package factory
