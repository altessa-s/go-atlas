// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package appstats provides high-performance application statistics monitoring.
// Collects CPU, memory, network I/O, and goroutine metrics with sub-microsecond latency.
//
// # One-off Statistics Collection
//
//	ctx := context.Background()
//	stats := appstats.GetApplicationStats(ctx)
//	fmt.Printf("CPU: %s, Memory: %s, Goroutines: %d\n",
//	    stats.CPU.Service, stats.Memory.Service, stats.Runtime.Goroutines)
//
// # Periodic Stats Logging with Scheduler
//
// For periodic logging of application statistics, use StatsLogger with service/scheduler:
//
//	statsLogger := appstats.NewStatsLogger(appstats.WithLogger(logger))
//	sched.Register(ctx, scheduler.TaskConfig{
//	    ID:         "appstats-logging",
//	    Schedule:   appstats.DefaultStatsLogSchedule, // every 5 minutes
//	    Func:       statsLogger.RunLogCycle,
//	    RunOnStart: true,
//	})
package appstats
