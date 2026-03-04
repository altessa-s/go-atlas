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
package factory
