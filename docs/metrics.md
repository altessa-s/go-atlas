# Metrics

All metrics are Prometheus-compatible and follow the naming convention
`{serviceName}_{subsystem}_{name}`. The `serviceName` prefix is configured
via `config.Metrics.ServiceName`.

**27 subsystems, 143 metrics.**

---

## audit

Package: `data/audit`

| Name                                 | Type      | Labels | Description                                      |
|--------------------------------------|-----------|--------|--------------------------------------------------|
| `audit_events_emitted_total`         | Counter   | --     | Audit events successfully emitted                |
| `audit_events_dropped_total`         | Counter   | --     | Audit events dropped due to a full buffer        |
| `audit_batch_flush_duration_seconds` | Histogram | --     | Batch flush operation duration                   |
| `audit_store_errors_total`           | Counter   | --     | Batch store failures after all retries           |
| `audit_workers_active`               | Gauge     | --     | Currently active dispatch workers                |

---

## broker

Package: `transport/broker`

| Name                                         | Type      | Labels    | Description                        |
|----------------------------------------------|-----------|-----------|------------------------------------|
| `broker_messages_published_total`            | Counter   | `subject` | Messages successfully published    |
| `broker_publish_errors_total`                | Counter   | `subject` | Message publish failures           |
| `broker_publish_duration_seconds`            | Histogram | --        | Publish operation duration         |
| `broker_messages_received_total`             | Counter   | `subject` | Messages received by subscribers   |
| `broker_message_processing_duration_seconds` | Histogram | `subject` | Message handler execution duration |
| `broker_message_processing_errors_total`     | Counter   | `subject` | Message handler failures           |

---

## broker_inprogress

Package: `transport/broker/inprogress`

| Name                                       | Type    | Labels | Description                              |
|--------------------------------------------|---------|--------|------------------------------------------|
| `broker_inprogress_heartbeats_sent_total`  | Counter | --     | InProgress heartbeats sent               |
| `broker_inprogress_heartbeat_errors_total` | Counter | --     | Failed InProgress heartbeat attempts     |

---

## budget_limiter

Package: `data/limiters/budget`

| Name                                         | Type    | Labels | Description                                      |
|----------------------------------------------|---------|--------|--------------------------------------------------|
| `budget_limiter_requests_allowed_total`      | Counter | --     | Requests allowed by the budget limiter           |
| `budget_limiter_requests_rejected_total`     | Counter | --     | Requests rejected due to budget exhaustion       |
| `budget_limiter_limit_check_errors_total`    | Counter | --     | Errors during budget limit checks                |

---

## cache

Package: `data/cache`

| Name                              | Type      | Labels       | Description                          |
|-----------------------------------|-----------|--------------|--------------------------------------|
| `cache_hits_total`                | Counter   | `cache_name` | Cache hits                           |
| `cache_misses_total`              | Counter   | `cache_name` | Cache misses                         |
| `cache_errors_total`              | Counter   | `cache_name` | Cache operation errors               |
| `cache_write_duration_seconds`    | Histogram | --           | Cache write operation duration       |
| `cache_fallback_duration_seconds` | Histogram | --           | Fallback function execution duration |
| `cache_evictions_total`           | Counter   | `cache_name` | Cache evictions                      |
| `cache_size`                      | Gauge     | `cache_name` | Current cache entries                |

---

## dlock

Package: `data/locks/dlock`

| Name                             | Type      | Labels | Description                     |
|----------------------------------|-----------|--------|---------------------------------|
| `dlock_locks_acquired_total`     | Counter   | --     | Locks successfully acquired     |
| `dlock_locks_failed_total`       | Counter   | --     | Lock acquisition failures       |
| `dlock_acquire_duration_seconds` | Histogram | --     | Lock acquisition duration       |
| `dlock_synchronizations_total`   | Counter   | --     | Synchronize calls completed     |

---

## filter

Package: `data/filter`

| Name                            | Type      | Labels           | Description                           |
|---------------------------------|-----------|------------------|---------------------------------------|
| `filter_parse_duration_seconds` | Histogram | --               | CEL expression parse duration         |
| `filter_parse_errors_total`     | Counter   | --               | CEL expression parse failures         |
| `filter_translations_total`     | Counter   | `target_backend` | Filter translations performed         |

---

## grpc_connection_pool

Package: `transport/grpc/client/pool`

| Name                                               | Type      | Labels | Description                            |
|----------------------------------------------------|-----------|--------|----------------------------------------|
| `grpc_connection_pool_connections_created_total`   | Counter   | --     | Connections created                    |
| `grpc_connection_pool_connections_closed_total`    | Counter   | --     | Connections closed                     |
| `grpc_connection_pool_connections_reused_total`    | Counter   | --     | Connections reused from the pool       |
| `grpc_connection_pool_connection_errors_total`     | Counter   | --     | Connection creation failures           |
| `grpc_connection_pool_connections_active`          | Gauge     | --     | Currently active connections           |
| `grpc_connection_pool_connections_in_use`          | Gauge     | --     | Connections currently in use           |
| `grpc_connection_pool_connections_idle`            | Gauge     | --     | Idle connections available             |
| `grpc_connection_pool_waiters`                     | Gauge     | --     | Goroutines waiting for a connection    |
| `grpc_connection_pool_connect_duration_seconds`    | Histogram | --     | Connection establishment duration      |
| `grpc_connection_pool_cleanup_duration_seconds`    | Histogram | --     | Cleanup cycle duration                 |
| `grpc_connection_pool_cleanup_connections_removed` | Counter   | --     | Connections removed during cleanup     |

---

## health

Package: `observability/health`

| Name                                  | Type      | Labels | Description                        |
|---------------------------------------|-----------|--------|------------------------------------|
| `health_check_cycle_duration_seconds` | Histogram | --     | Health check cycle duration        |
| `health_status_changes_total`         | Counter   | --     | Health status changes detected     |
| `health_checks_performed_total`       | Counter   | --     | Individual health checks performed |

---

## http_client

Package: `transport/http/client`

| Name                                      | Type      | Labels                   | Description                                           |
|-------------------------------------------|-----------|--------------------------|-------------------------------------------------------|
| `http_client_requests_total`              | Counter   | `method`, `status_class` | HTTP requests completed                               |
| `http_client_request_errors_total`        | Counter   | `method`                 | HTTP request errors after all retries                 |
| `http_client_retries_total`               | Counter   | --                       | HTTP request retry attempts                           |
| `http_client_request_duration_seconds`    | Histogram | `method`                 | HTTP request duration including retries               |
| `http_client_circuit_breaker_trips_total` | Counter   | --                       | Circuit breaker trip events                           |
| `http_client_circuit_breaker_state`       | Gauge     | `host`                   | Circuit breaker state (0=closed, 1=half-open, 2=open) |

---

## idempotency

Package: `data/idempotency`

| Name                               | Type    | Labels | Description                              |
|------------------------------------|---------|--------|------------------------------------------|
| `idempotency_locks_acquired_total` | Counter | --     | Idempotency keys successfully locked     |
| `idempotency_locks_denied_total`   | Counter | --     | Duplicate requests detected              |
| `idempotency_completions_total`    | Counter | --     | Idempotency keys marked as complete      |
| `idempotency_deletions_total`      | Counter | --     | Idempotency keys deleted                 |
| `idempotency_errors_total`         | Counter | --     | Idempotency operation errors             |

---

## interner

Package: `observability/metrics` (bridge for `core/text/strings`)

| Name                       | Type    | Labels | Description                                          |
|----------------------------|---------|--------|------------------------------------------------------|
| `interner_hot_hits_total`  | Counter | --     | Lookups served from the hot cache (atomic slots)     |
| `interner_cold_hits_total` | Counter | --     | Lookups served from the cold cache (sync.Map)        |
| `interner_misses_total`    | Counter | --     | Lookups requiring a new entry                        |
| `interner_evictions_total` | Counter | --     | Entries removed by background LRU eviction           |
| `interner_current_size`    | Gauge   | --     | Currently interned strings                           |
| `interner_hit_rate`        | Gauge   | --     | Overall cache hit rate (hot + cold / total)          |
| `interner_hot_hit_rate`    | Gauge   | --     | Hot cache hit rate (hot / total)                     |

---

## leader_election

Package: `data/leadelect`

| Name                                        | Type      | Labels | Description                                     |
|---------------------------------------------|-----------|--------|-------------------------------------------------|
| `leader_election_transitions_total`         | Counter   | `type` | Leadership transitions                          |
| `leader_election_is_leader`                 | Gauge     | --     | Current leader status (1=leader, 0=follower)    |
| `leader_election_callback_duration_seconds` | Histogram | --     | Leadership callback execution duration          |
| `leader_election_callback_errors_total`     | Counter   | --     | Failed leadership callback executions           |

---

## mongo

Package: `data/mongo`

| Name                                 | Type      | Labels              | Description                        |
|--------------------------------------|-----------|---------------------|------------------------------------|
| `mongo_connect_duration_seconds`     | Histogram | --                  | MongoDB connect duration           |
| `mongo_ping_retries_total`           | Counter   | --                  | MongoDB ping retry attempts        |
| `mongo_transaction_duration_seconds` | Histogram | `collection`        | MongoDB transaction duration       |
| `mongo_transaction_errors_total`     | Counter   | --                  | Failed MongoDB transactions        |
| `mongo_operations_total`             | Counter   | `op`, `collection`  | MongoDB CRUD operations            |
| `mongo_operation_duration_seconds`   | Histogram | `op`, `collection`  | MongoDB operation duration         |

---

## nats_kv_lease

Package: `data/internal/natskvlease`

| Name                             | Type    | Labels | Description                                    |
|----------------------------------|---------|--------|------------------------------------------------|
| `nats_kv_lease_operations_total` | Counter | --     | Lease operations                               |
| `nats_kv_lease_lease_held`       | Gauge   | --     | Lease held status (1=held, 0=not held)         |

---

## nats_leader_election

Package: `data/leadelect/providers/nats`

| Name                                                      | Type      | Labels | Description                                     |
|-----------------------------------------------------------|-----------|--------|-------------------------------------------------|
| `nats_leader_election_lease_operations_total`             | Counter   | --     | Lease operations                                |
| `nats_leader_election_is_leader`                          | Gauge     | --     | Current leader status (1=leader, 0=follower)    |
| `nats_leader_election_camping_iteration_duration_seconds` | Histogram | --     | Camping loop iteration duration                 |

---

## oidc

Package: `auth/oidc`

| Name                                 | Type      | Labels   | Description                          |
|--------------------------------------|-----------|----------|--------------------------------------|
| `oidc_token_validations_total`       | Counter   | `issuer` | Token validation attempts            |
| `oidc_validation_errors_total`       | Counter   | `issuer` | Token validation errors              |
| `oidc_revocation_check_errors_total` | Counter   | --       | Token revocation check failures      |
| `oidc_validation_duration_seconds`   | Histogram | --       | Token validation duration            |
| `oidc_cache_hits_total`              | Counter   | --       | Token cache hits                     |
| `oidc_cache_misses_total`            | Counter   | --       | Token cache misses                   |
| `oidc_jwks_refreshes_total`          | Counter   | --       | JWKS refresh operations              |
| `oidc_jwks_refresh_errors_total`     | Counter   | --       | Failed JWKS refresh operations       |
| `oidc_jwks_refresh_duration_seconds` | Histogram | --       | JWKS refresh duration                |

---

## opa

Package: `auth/opa`

| Name                                 | Type      | Labels   | Description                     |
|--------------------------------------|-----------|----------|---------------------------------|
| `opa_policy_reloads_total`           | Counter   | `result` | Policy reload operations        |
| `opa_policy_reload_duration_seconds` | Histogram | --       | Policy reload duration          |
| `opa_evaluations_total`              | Counter   | `result` | Policy evaluations              |
| `opa_evaluation_duration_seconds`    | Histogram | --       | Policy evaluation duration      |
| `opa_modules_loaded`                 | Gauge     | --       | Policy modules currently loaded |

---

## outbox

Package: `data/outbox`

| Name                                     | Type      | Labels | Description                               |
|------------------------------------------|-----------|--------|-------------------------------------------|
| `outbox_events_dispatched_total`         | Counter   | --     | Events successfully dispatched            |
| `outbox_events_dispatch_errors_total`    | Counter   | --     | Event dispatch failures after all retries |
| `outbox_events_saved_total`              | Counter   | --     | Events persisted to the outbox store      |
| `outbox_events_skipped_total`            | Counter   | --     | Events skipped due to key compaction      |
| `outbox_events_in_flight`                | Gauge     | --     | Events currently being dispatched         |
| `outbox_dispatch_retries_total`          | Counter   | --     | Event dispatch retry attempts             |
| `outbox_max_retries_exhausted_total`     | Counter   | --     | Events hitting the retry limit            |
| `outbox_dispatch_cycle_duration_seconds` | Histogram | --     | Dispatch cycle duration                   |
| `outbox_unlock_cycle_duration_seconds`   | Histogram | --     | Unlock cycle duration                     |
| `outbox_cleanup_cycle_duration_seconds`  | Histogram | --     | Cleanup cycle duration                    |

---

## probfilter

Package: `data/probfilter`

| Name                                  | Type      | Labels                  | Description                          |
|---------------------------------------|-----------|-------------------------|--------------------------------------|
| `probfilter_lookups_total`            | Counter   | `filter_name`, `result` | Probabilistic filter lookups         |
| `probfilter_adds_total`               | Counter   | `filter_name`           | Items added to probabilistic filters |
| `probfilter_lookup_duration_seconds`  | Histogram | `filter_name`           | Filter lookup duration               |
| `probfilter_rebuild_duration_seconds` | Histogram | --                      | Filter rebuild duration              |
| `probfilter_rebuild_errors_total`     | Counter   | --                      | Failed filter rebuild operations     |

---

## rate_limiter

Package: `data/limiters/tokenbucket`

| Name                                    | Type    | Labels | Description                          |
|-----------------------------------------|---------|--------|--------------------------------------|
| `rate_limiter_requests_allowed_total`   | Counter | --     | Requests allowed by the rate limiter |
| `rate_limiter_requests_rejected_total`  | Counter | --     | Requests rejected due to rate limit  |
| `rate_limiter_limit_check_errors_total` | Counter | --     | Errors during rate limit checks      |

---

## regex_cache

Package: `observability/metrics` (bridge for `data/filter`)

| Name                       | Type    | Labels | Description                                      |
|----------------------------|---------|--------|--------------------------------------------------|
| `regex_cache_hits_total`   | Counter | --     | Regex pattern cache hits                         |
| `regex_cache_misses_total` | Counter | --     | Regex pattern cache misses requiring compilation |
| `regex_cache_current_size` | Gauge   | --     | Compiled regex patterns in the cache             |
| `regex_cache_hit_rate`     | Gauge   | --     | Regex pattern cache hit rate                     |

---

## scheduler

Package: `service/scheduler`

| Name                                    | Type      | Labels                | Description                                  |
|-----------------------------------------|-----------|-----------------------|----------------------------------------------|
| `scheduler_tasks_dispatched_total`      | Counter   | `task_id`, `priority` | Task executions started                      |
| `scheduler_task_duration_seconds`       | Histogram | `task_id`, `priority` | Task execution duration                      |
| `scheduler_task_errors_total`           | Counter   | `task_id`             | Failed task executions                       |
| `scheduler_tasks_skipped_total`         | Counter   | `task_id`             | Executions skipped via SkipNextRun           |
| `scheduler_stale_tasks_recovered_total` | Counter   | --                    | Stale task recoveries                        |
| `scheduler_tick_duration_seconds`       | Histogram | --                    | Tick cycle duration                          |
| `scheduler_tasks_running`               | Gauge     | --                    | Currently executing tasks                    |
| `scheduler_tasks_registered`            | Gauge     | --                    | Registered tasks                             |
| `scheduler_dispatch_lag_seconds`        | Histogram | `task_id`             | Scheduled vs actual execution delay          |
| `scheduler_storage_errors_total`        | Counter   | `op`                  | Storage operation failures                   |

---

## secrets

Package: `security/secrets`

| Name                                    | Type      | Labels | Description                   |
|-----------------------------------------|-----------|--------|-------------------------------|
| `secrets_cache_hits_total`              | Counter   | --     | Cache hits                    |
| `secrets_cache_misses_total`            | Counter   | --     | Cache misses                  |
| `secrets_fetch_duration_seconds`        | Histogram | --     | Secret fetch duration         |
| `secrets_update_cycle_duration_seconds` | Histogram | --     | Cache update cycle duration   |
| `secrets_update_cycle_errors_total`     | Counter   | --     | Failed update cycles          |
| `secrets_cache_size`                    | Gauge     | --     | Current cache entries         |

---

## uniq

Package: `data/uniq`

| Name                              | Type      | Labels | Description                         |
|-----------------------------------|-----------|--------|-------------------------------------|
| `uniq_operations_total`           | Counter   | `op`   | Uniqueness check operations         |
| `uniq_operation_duration_seconds` | Histogram | `op`   | Uniqueness check operation duration |
| `uniq_operation_errors_total`     | Counter   | `op`   | Uniqueness check operation failures |

---

## vault

Package: `security/vault`

| Name                             | Type      | Labels | Description                      |
|----------------------------------|-----------|--------|----------------------------------|
| `vault_renewal_attempts_total`   | Counter   | --     | Vault token renewal attempts     |
| `vault_renewal_errors_total`     | Counter   | --     | Vault token renewal failures     |
| `vault_renewal_duration_seconds` | Histogram | --     | Vault token renewal duration     |

---

## vault_auth

Package: `security/vault/auth`

| Name                                    | Type    | Labels | Description                       |
|-----------------------------------------|---------|--------|-----------------------------------|
| `vault_auth_auth_attempts_total`        | Counter | --     | Authentication attempts           |
| `vault_auth_auth_errors_total`          | Counter | `type` | Authentication errors             |
| `vault_auth_token_renewals_total`       | Counter | --     | Successful token renewals         |
| `vault_auth_token_renewal_errors_total` | Counter | --     | Token renewal errors              |
