// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package health provides transport-agnostic health check coordination and monitoring.
// It manages the health status of various system components (databases, caches, services)
// and handles subscriptions for status changes independent of any transport protocol.
//
// # Architecture
//
// The central type is [Coordinator], which holds a registry of named services
// (each implementing [Checker]) and a set of watcher subscriptions ([Subscriber]).
// Status checks are cached for a configurable TTL ([WithStatusCacheTTL]) and
// executed with a per-check timeout ([WithCheckTimeout]).
//
// Watcher channels are sharded across [WithNumShards] shards to reduce lock
// contention under high fan-out. Buffer sizes adapt automatically when the
// active watcher count exceeds [WithAdaptiveBufferThreshold].
//
// # Features
//
//   - Unified Monitoring: single registry for all system health checks.
//   - Transport Independence: status results can be exposed via HTTP, gRPC, or CLI.
//   - Reactive Subscriptions: [Coordinator.Subscribe] delivers [ServingStatus] changes.
//   - Scheduler Integration: [WithScheduler] and [WithCheckSchedule] automate polling.
//   - Concurrency Safe: all methods on [Coordinator] are safe for concurrent use.
//
// # Usage
//
//	coordinator := health.New()
//
//	// Register a simple component check
//	coordinator.RegisterService("database", health.Func(func(ctx context.Context) health.ServingStatus {
//	    if err := db.PingContext(ctx); err != nil {
//	        return health.StatusNotServing
//	    }
//	    return health.StatusServing
//	}))
//
//	// Register periodic health checks with scheduler
//	sched.Register(ctx, scheduler.TaskConfig{
//	    ID:         "health-check",
//	    Schedule:   "*/5 * * * * *", // every 5 seconds
//	    Func:       coordinator.RunHealthCheckCycle,
//	    RunOnStart: true,
//	})
//
//	// Check status
//	status := coordinator.CheckStatus(ctx, "database")
package health
