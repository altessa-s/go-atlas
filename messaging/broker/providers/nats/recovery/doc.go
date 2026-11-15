// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package recovery provides automatic recovery for NATS JetStream streams and consumers.
//
// The package uses JetStream Advisory Events as the primary detection mechanism
// for stream/consumer deletions and periodic health checks as a fallback.
//
// # Architecture
//
// The package consists of several components:
//
//   - [Manager]: Main orchestrator that composes all recovery components
//   - [Registry]: Thread-safe storage for stream configurations and subscription data
//   - [Supervisor]: Handles recovery logic with exponential backoff
//   - [AdvisoryListener]: Subscribes to JetStream advisory events for instant detection
//   - [HealthMonitor]: Fallback checks for missed events (scheduler-managed)
//   - [RecoveryTimeoutMonitor]: Clears stale "recovering" marks (scheduler-managed)
//
// # Integration with service/scheduler
//
// Background tasks (health checks, stale recovery cleanup) are designed to be run
// via an external scheduler ([service/scheduler]). This provides:
//
//   - Unified task management across the application
//   - Pause/resume/disable capabilities
//   - Execution history and monitoring
//   - Leader election for distributed deployments
//
// # Integration with natsprovider
//
// The recovery package integrates with the existing [natsprovider] package by accepting
// [broker.SubscriberFactory] for subscription configuration. This design:
//
//   - Eliminates duplicated message handling logic
//   - Reuses existing, tested subscription code
//   - Ensures consistent behavior with direct natsprovider usage
//
// # Recovery Strategies
//
// Three recovery strategies are supported via [RecoveryStrategy]:
//
//   - [RecoveryStrategyAuto]: Automatic recovery with exponential backoff (default)
//   - [RecoveryStrategyManual]: Notify via callback, no auto-recovery
//   - [RecoveryStrategySkip]: Ignore deletion events completely
//
// # Configuration Options
//
// The Manager accepts functional options:
//
//   - [WithMaxRecoveryAttempts]: Max recovery attempts before giving up (default: 3)
//   - [WithRecoveryBackoff]: Base backoff between attempts (default: 5s)
//   - [WithStaleRecoveryTimeout]: When to clear stale recovery marks (default: 20m)
//   - [WithLogger]: Logger for recovery operations
//   - [WithOnRecoverySuccess]: Callback on successful recovery
//   - [WithOnRecoveryFailure]: Callback on failed recovery
//   - [WithOnManualRecoveryNeeded]: Callback for manual strategy streams
//   - [WithOnStaleRecoveryCleared]: Callback when stale mark cleared
//
// # Usage
//
//	// Create NATS provider
//	provider, _ := natsprovider.New(natsConn)
//
//	// Create recovery manager
//	manager, _ := recovery.New(provider,
//	    recovery.WithMaxRecoveryAttempts(3),
//	    recovery.WithLogger(logger),
//	)
//
//	// Start advisory listener for instant detection
//	manager.Start()
//
//	// Register background tasks with external scheduler
//	scheduler.Register(ctx, scheduler.TaskConfig{
//	    ID:       "recovery-health-check",
//	    Schedule: "0 */5 * * * *", // every 5 minutes
//	    Func:     manager.RunHealthCheckCycle,
//	})
//
//	scheduler.Register(ctx, scheduler.TaskConfig{
//	    ID:       "recovery-stale-cleanup",
//	    Schedule: "0 * * * * *", // every minute
//	    Func:     manager.RunStaleRecoveryCleanup,
//	})
//
//	// Register stream for recovery
//	manager.RegisterStream(jetstream.StreamConfig{
//	    Name:     "ORDERS",
//	    Subjects: []string{"orders.>"},
//	}, recovery.WithRecoveryStrategy(recovery.RecoveryStrategyAuto))
//
//	// Subscribe using natsprovider factory - recovery is automatic
//	sub, err := manager.Subscribe(ctx, "ORDERS", "order-processor", handler,
//	    natsprovider.SubscriberWithConsumer(&jetstream.ConsumerConfig{
//	        Durable: "order-processor",
//	    }),
//	)
//
//	// On shutdown
//	manager.Close()
//
// # Thread Safety
//
// All components are thread-safe for concurrent use. The Manager uses mutex-protected
// maps and atomic operations for state management.
package recovery
