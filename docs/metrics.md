# Metrics Reference

All metrics are Prometheus-compatible and follow the naming convention
`{serviceName}_{subsystem}_{name}`. The `serviceName` prefix is configured
via `config.Metrics.ServiceName`. This document lists every metric registered
across the 20 instrumented subsystems (98 metrics total).

---

## audit

Package: `data/audit`

| Name | Type | Description |
|------|------|-------------|
| `audit_events_emitted_total` | Counter | Total number of audit events successfully emitted. |
| `audit_events_dropped_total` | Counter | Total number of audit events dropped due to a full buffer. |
| `audit_batch_flush_duration_seconds` | Histogram | Duration of batch flush operations in seconds. |
| `audit_store_errors_total` | Counter | Total number of batch store failures after all retries. |
| `audit_workers_active` | Gauge | Number of currently active dispatch workers. |

## broker

Package: `transport/broker`

| Name | Type | Description |
|------|------|-------------|
| `broker_messages_published_total` | Counter | Total number of messages successfully published. |
| `broker_publish_errors_total` | Counter | Total number of message publish failures. |
| `broker_publish_duration_seconds` | Histogram | Duration of publish operations in seconds. |

## cache

Package: `data/cache`

| Name | Type | Description |
|------|------|-------------|
| `cache_hits_total` | Counter | Total number of cache hits. |
| `cache_misses_total` | Counter | Total number of cache misses. |
| `cache_errors_total` | Counter | Total number of cache operation errors. |
| `cache_write_duration_seconds` | Histogram | Duration of cache write operations in seconds. |
| `cache_fallback_duration_seconds` | Histogram | Duration of fallback function execution in seconds. |

## dlock

Package: `data/locks/dlock`

| Name | Type | Description |
|------|------|-------------|
| `dlock_locks_acquired_total` | Counter | Total number of locks successfully acquired. |
| `dlock_locks_failed_total` | Counter | Total number of lock acquisition failures. |
| `dlock_acquire_duration_seconds` | Histogram | Duration of lock acquisition in seconds. |
| `dlock_synchronizations_total` | Counter | Total number of Synchronize calls completed. |

## grpc_connection_pool

Package: `transport/grpc/client/pool`

| Name | Type | Description |
|------|------|-------------|
| `grpc_connection_pool_connections_created_total` | Counter | Total number of connections created. |
| `grpc_connection_pool_connections_closed_total` | Counter | Total number of connections closed. |
| `grpc_connection_pool_connections_reused_total` | Counter | Total number of connections reused from the pool. |
| `grpc_connection_pool_connection_errors_total` | Counter | Total number of connection creation failures. |
| `grpc_connection_pool_connections_active` | Gauge | Number of currently active connections. |
| `grpc_connection_pool_connections_in_use` | Gauge | Number of connections currently in use by callers. |
| `grpc_connection_pool_connections_idle` | Gauge | Number of idle connections available in the pool. |
| `grpc_connection_pool_connect_duration_seconds` | Histogram | Duration of connection establishment in seconds. |
| `grpc_connection_pool_cleanup_duration_seconds` | Histogram | Duration of cleanup cycles in seconds. |
| `grpc_connection_pool_cleanup_connections_removed` | Counter | Total number of connections removed during cleanup. |

## health

Package: `observability/health`

| Name | Type | Description |
|------|------|-------------|
| `health_check_cycle_duration_seconds` | Histogram | Duration of a single health check cycle in seconds. |
| `health_status_changes_total` | Counter | Total number of health status changes detected. |
| `health_checks_performed_total` | Counter | Total number of individual health checks performed. |

## http_client

Package: `transport/http/client`

| Name | Type | Description |
|------|------|-------------|
| `http_client_requests_total` | Counter | Total number of HTTP requests completed. |
| `http_client_request_errors_total` | Counter | Total number of HTTP request errors after all retries. |
| `http_client_retries_total` | Counter | Total number of HTTP request retry attempts. |
| `http_client_request_duration_seconds` | Histogram | Duration of HTTP requests including retries in seconds. |
| `http_client_circuit_breaker_trips_total` | Counter | Total number of circuit breaker trip events. |

## idempotency

Package: `data/idempotency`

| Name | Type | Description |
|------|------|-------------|
| `idempotency_locks_acquired_total` | Counter | Total number of idempotency keys successfully locked. |
| `idempotency_locks_denied_total` | Counter | Total number of duplicate requests detected. |
| `idempotency_completions_total` | Counter | Total number of idempotency keys marked as complete. |
| `idempotency_deletions_total` | Counter | Total number of idempotency keys deleted. |
| `idempotency_errors_total` | Counter | Total number of idempotency operation errors. |

## leader_election

Package: `data/leadelect`

| Name | Type | Description |
|------|------|-------------|
| `leader_election_transitions_total` | Counter | Total number of leadership transitions. |
| `leader_election_is_leader` | Gauge | Whether this instance is the current leader (1=leader, 0=follower). |
| `leader_election_callback_duration_seconds` | Histogram | Duration of leadership callback executions in seconds. |
| `leader_election_callback_errors_total` | Counter | Total number of failed leadership callback executions. |

## mongo

Package: `data/mongo`

| Name | Type | Description |
|------|------|-------------|
| `mongo_connect_duration_seconds` | Histogram | Duration of MongoDB connect operations in seconds. |
| `mongo_ping_retries_total` | Counter | Total number of MongoDB ping retry attempts. |
| `mongo_transaction_duration_seconds` | Histogram | Duration of MongoDB transactions in seconds. |
| `mongo_transaction_errors_total` | Counter | Total number of failed MongoDB transactions. |

## nats_kv_lease

Package: `data/internal/natskvlease`

| Name | Type | Description |
|------|------|-------------|
| `nats_kv_lease_operations_total` | Counter | Total number of lease operations. |
| `nats_kv_lease_lease_held` | Gauge | Whether the lease is currently held (1=held, 0=not held). |

## nats_leader_election

Package: `data/leadelect/providers/nats`

| Name | Type | Description |
|------|------|-------------|
| `nats_leader_election_lease_operations_total` | Counter | Total number of lease operations. |
| `nats_leader_election_is_leader` | Gauge | Whether this instance is the current leader (1=leader, 0=follower). |
| `nats_leader_election_camping_iteration_duration_seconds` | Histogram | Duration of camping loop iterations in seconds. |

## oidc

Package: `auth/oidc`

| Name | Type | Description |
|------|------|-------------|
| `oidc_token_validations_total` | Counter | Total number of token validation attempts. |
| `oidc_validation_errors_total` | Counter | Total number of token validation errors. |
| `oidc_validation_duration_seconds` | Histogram | Duration of token validation operations in seconds. |
| `oidc_cache_hits_total` | Counter | Total number of token cache hits. |
| `oidc_cache_misses_total` | Counter | Total number of token cache misses. |
| `oidc_jwks_refreshes_total` | Counter | Total number of JWKS refresh operations. |
| `oidc_jwks_refresh_errors_total` | Counter | Total number of failed JWKS refresh operations. |
| `oidc_jwks_refresh_duration_seconds` | Histogram | Duration of JWKS refresh operations in seconds. |

## opa

Package: `auth/opa`

| Name | Type | Description |
|------|------|-------------|
| `opa_policy_reloads_total` | Counter | Total number of policy reload operations. |
| `opa_policy_reload_duration_seconds` | Histogram | Duration of policy reload operations in seconds. |
| `opa_evaluations_total` | Counter | Total number of policy evaluations. |
| `opa_evaluation_duration_seconds` | Histogram | Duration of policy evaluations in seconds. |
| `opa_modules_loaded` | Gauge | Number of policy modules currently loaded. |

## outbox

Package: `data/outbox`

| Name | Type | Description |
|------|------|-------------|
| `outbox_events_dispatched_total` | Counter | Total number of events successfully dispatched. |
| `outbox_events_dispatch_errors_total` | Counter | Total number of event dispatch failures after all retries. |
| `outbox_events_saved_total` | Counter | Total number of events persisted to the outbox store. |
| `outbox_events_skipped_total` | Counter | Total number of events skipped due to key compaction. |
| `outbox_events_in_flight` | Gauge | Number of events currently being dispatched. |
| `outbox_dispatch_cycle_duration_seconds` | Histogram | Duration of a single dispatch cycle in seconds. |
| `outbox_unlock_cycle_duration_seconds` | Histogram | Duration of a single unlock cycle in seconds. |
| `outbox_cleanup_cycle_duration_seconds` | Histogram | Duration of a single cleanup cycle in seconds. |

## rate_limiter

Package: `data/limiters/tokenbucket`

| Name | Type | Description |
|------|------|-------------|
| `rate_limiter_requests_allowed_total` | Counter | Total number of requests allowed by the rate limiter. |
| `rate_limiter_requests_rejected_total` | Counter | Total number of requests rejected due to rate limiting. |
| `rate_limiter_limit_check_errors_total` | Counter | Total number of errors during rate limit checks. |

## scheduler

Package: `service/scheduler`

| Name | Type | Description |
|------|------|-------------|
| `scheduler_tasks_dispatched_total` | Counter | Total number of task executions started. |
| `scheduler_task_duration_seconds` | Histogram | Duration of task executions in seconds. |
| `scheduler_task_errors_total` | Counter | Total number of failed task executions. |
| `scheduler_tasks_skipped_total` | Counter | Total number of task executions skipped via SkipNextRun. |
| `scheduler_stale_tasks_recovered_total` | Counter | Total number of stale task recoveries. |
| `scheduler_tick_duration_seconds` | Histogram | Duration of each scheduler tick cycle in seconds. |
| `scheduler_tasks_running` | Gauge | Number of currently executing tasks. |
| `scheduler_tasks_registered` | Gauge | Total number of registered tasks. |

## secrets

Package: `security/secrets`

| Name | Type | Description |
|------|------|-------------|
| `secrets_cache_hits_total` | Counter | Total number of cache hits. |
| `secrets_cache_misses_total` | Counter | Total number of cache misses. |
| `secrets_fetch_duration_seconds` | Histogram | Duration of secret fetch operations in seconds. |
| `secrets_update_cycle_duration_seconds` | Histogram | Duration of cache update cycles in seconds. |
| `secrets_update_cycle_errors_total` | Counter | Total number of failed update cycles. |
| `secrets_cache_size` | Gauge | Current number of entries in the cache. |

## vault

Package: `security/vault`

| Name | Type | Description |
|------|------|-------------|
| `vault_renewal_attempts_total` | Counter | Total number of Vault token renewal attempts. |
| `vault_renewal_errors_total` | Counter | Total number of Vault token renewal failures. |
| `vault_renewal_duration_seconds` | Histogram | Duration of Vault token renewal operations in seconds. |

## vault_auth

Package: `security/vault/auth`

| Name | Type | Description |
|------|------|-------------|
| `vault_auth_auth_attempts_total` | Counter | Total number of authentication attempts. |
| `vault_auth_auth_errors_total` | Counter | Total number of authentication errors. |
| `vault_auth_token_renewals_total` | Counter | Total number of successful token renewals. |
| `vault_auth_token_renewal_errors_total` | Counter | Total number of token renewal errors. |
