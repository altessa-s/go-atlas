# recovery

```go
import "github.com/altessa-s/go-atlas/transport/broker/providers/nats/recovery"
```

Package `recovery` provides automatic recovery for NATS JetStream streams and consumers. Uses JetStream Advisory Events for instant deletion detection
and periodic health checks as a fallback for missed events during reconnection or network issues.

## Components

| Type                     | Description                                                                     |
|--------------------------|---------------------------------------------------------------------------------|
| `Manager`                | Main orchestrator composing all recovery components; safe for concurrent use    |
| `Registry`               | Thread-safe storage for stream configurations and subscription data             |
| `Supervisor`             | Handles recovery with linear backoff; guards concurrent stream recovery, but does not deduplicate consumer recovery triggers       |
| `AdvisoryListener`       | Subscribes to JetStream advisory events for instant deletion detection          |
| `HealthMonitor`          | Fallback checks for missed events; designed for scheduler-managed execution     |
| `RecoveryTimeoutMonitor` | Clears stale recovery marks to prevent permanent recovery blockage              |
| `ManagedSubscription`    | Subscription with automatic consumer recovery on deletion                       |

## Recovery strategies

| Strategy                  | Description                                                                    |
|---------------------------|--------------------------------------------------------------------------------|
| `RecoveryStrategyAuto`    | Automatic recovery with linear backoff (default)                               |
| `RecoveryStrategyManual`  | Notify via `OnManualRecoveryNeeded` callback; no automatic recovery            |
| `RecoveryStrategySkip`    | Ignore deletion events entirely; no recovery and no notification               |

## Manager methods

| Method                    | Description                                                                    |
|---------------------------|--------------------------------------------------------------------------------|
| `New`                     | Create a new `Manager` with a NATS provider and options                        |
| `Start`                   | Begin the advisory listener for instant deletion detection                     |
| `Close`                   | Stop all components and drain active subscriptions                             |
| `RegisterStream`          | Register a stream configuration for automatic recovery                         |
| `UnregisterStream`        | Remove a stream from recovery management                                       |
| `CreateStream`            | Create or update a JetStream stream and register it for recovery               |
| `Subscribe`               | Create a managed subscription with automatic consumer recovery                 |
| `RunHealthCheckCycle`     | Execute a single health check cycle (register with scheduler)                  |
| `RunStaleRecoveryCleanup` | Execute a single stale recovery cleanup cycle (register with scheduler)        |
| `RegisterHealthCheckSchedulerFunc` | Return the health check task for a scheduler; direct `RunHealthCheckCycle` calls then return `corescheduler.ErrSchedulerManaged` |
| `RegisterStaleCleanupSchedulerFunc` | Return the stale cleanup task for a scheduler; direct `RunStaleRecoveryCleanup` calls then return `corescheduler.ErrSchedulerManaged` |
| `GetRegistry`             | Return the internal `Registry` (testing and advanced use)                      |
| `GetSupervisor`           | Return the internal `Supervisor` (testing and advanced use)                    |

## Options

| Option                              | Description                                                       |
|-------------------------------------|-------------------------------------------------------------------|
| `WithLogger`                        | Set the `*slog.Logger` for recovery operations                    |
| `WithMaxRecoveryAttempts`           | Max recovery attempts before giving up (default: 3)               |
| `WithRecoveryBackoff`              | Base backoff duration between attempts (default: 5s)              |
| `WithStaleRecoveryTimeout`         | Timeout after which a recovery mark is cleared (default: 20m)     |
| `WithScheduler`                     | Set the task scheduler for background task registration           |
| `WithHealthCheckSchedule`          | Cron schedule for periodic health checks                          |
| `WithStaleRecoveryCleanupSchedule` | Cron schedule for stale recovery mark cleanup                     |
| `WithOnRecoverySuccess`            | Callback invoked after successful stream or consumer recovery     |
| `WithOnRecoveryFailure`            | Callback invoked when recovery fails after all retry attempts     |
| `WithOnManualRecoveryNeeded`       | Callback for streams configured with `RecoveryStrategyManual`     |
| `WithOnStaleRecoveryCleared`       | Callback when a stale recovery mark is cleared                    |

## Stream options

| Option                  | Description                                                                   |
|-------------------------|-------------------------------------------------------------------------------|
| `WithRecoveryStrategy`  | Set the `RecoveryStrategy` for a registered stream (default: Auto)            |

## Defaults

| Constant                            | Value  | Description                                             |
|-------------------------------------|--------|---------------------------------------------------------|
| `DefaultMaxRecoveryAttempts`        | 3      | Maximum recovery attempts before giving up              |
| `DefaultRecoveryBackoff`            | 5s     | Base backoff duration between recovery attempts         |
| `DefaultStaleRecoveryTimeout`       | 20m    | Timeout for clearing stale recovery marks               |

## Errors

| Error                            | Description                                                         |
|----------------------------------|---------------------------------------------------------------------|
| `ErrStreamNotRegistered`         | The stream is not registered for recovery                           |
| `ErrConsumerNotRegistered`       | The consumer is not registered for recovery                         |
| `ErrStreamAlreadyRegistered`     | Declared but currently unused; registering an existing stream updates its configuration                                    |
| `ErrRecoveryInProgress`          | Declared but currently unused; duplicate stream recovery requests are silently ignored            |
| `ErrMaxRecoveryAttemptsExceeded` | Declared but currently unused; exhausted recovery passes the actual retry error to `OnRecoveryFailure`           |
| `ErrManagerClosed`               | The manager has been closed                                         |
| `ErrNilJetStream`                | A JetStream instance is required                                    |
| `ErrNilNATSConn`                 | A NATS connection is required                                       |
| `ErrInvalidStreamConfig`         | The stream configuration is invalid                                 |
| `ErrInvalidConsumerConfig`       | The consumer configuration is invalid                               |
