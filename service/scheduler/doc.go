// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package scheduler provides an internal task scheduler for periodic job execution.
// It allows services and plugins to register tasks with configurable intervals,
// priorities, and concurrency limits.
//
// Example with static concurrency:
//
//	storage := memory.New(100)
//	s := scheduler.New(storage,
//	    scheduler.WithTickInterval(time.Second),
//	    scheduler.WithMaxConcurrentTasks(10),
//	)
//
//	if err := s.Start(ctx); err != nil {
//	    log.Fatal(err)
//	}
//	defer s.Stop(ctx)
//
//	err := s.Register(ctx, corescheduler.TaskConfig{
//	    ID:         "my-task",
//	    Schedule:   "@every 5m",
//	    Priority:   corescheduler.TaskPriorityHigh,
//	    RunOnStart: true,
//	    Func: func(ctx context.Context) error {
//	        return doWork(ctx)
//	    },
//	})
//
// For adaptive concurrency that reacts to runtime conditions, use
// WithConcurrencyLimitFunc or WithEnvironment:
//
//	s := scheduler.New(storage,
//	    scheduler.WithConcurrencyLimitFunc(
//	        concurrency.AdaptiveConcurrency(concurrency.AdaptiveConcurrencyConfig{
//	            MemoryLowThresholdMB:    256,
//	            MemoryMediumThresholdMB: 512,
//	        }),
//	    ),
//	)
//
// Or use a predefined environment profile:
//
//	s := scheduler.New(storage,
//	    scheduler.WithEnvironment(concurrency.EnvironmentIOBound),
//	)
//
// For distributed environments, use WithLeaderElector to ensure tasks run on
// only one node:
//
//	le, _ := leadelect.NewWithNats(conn, cfg)
//	s := scheduler.New(storage, scheduler.WithLeaderElector(le))
package scheduler
