# Metrics

All metrics are Prometheus-compatible and follow the naming convention `{serviceName}_{subsystem}_{name}`. The `serviceName` prefix is configured via
`observabilityconfig.Metrics.ServiceName`.

**39 subsystems, 227 metrics.**

---

## async

Package: `service/dispatch`

The subsystem defaults to `DefaultMetricsSubsystem` (`async`) and can be changed with `WithMetricsSubsystem` or the dispatch config.

| Name                                 | Type      | Labels | Description                               |
|--------------------------------------|-----------|--------|-------------------------------------------|
| `async_items_enqueued_total`         | Counter   | --     | Items accepted into the dispatch buffer   |
| `async_items_dropped_total`          | Counter   | --     | Items dropped (buffer full or shutdown)   |
| `async_batch_flush_duration_seconds` | Histogram | --     | Batch flush duration                      |
| `async_sink_errors_total`            | Counter   | --     | Batches the sink failed after all retries |
| `async_wal_errors_total`             | Counter   | --     | WAL append/ack failures                   |
| `async_encode_errors_total`          | Counter   | --     | Items the codec failed to encode          |
| `async_workers_active`               | Gauge     | --     | Currently active dispatch workers         |
| `async_wal_bytes`                    | Gauge     | --     | WAL size on disk                          |
| `async_wal_replay_total`             | Counter   | --     | Items replayed from the WAL on start      |

---

## audit

Package: `data/audit`

| Name                         | Type    | Labels | Description                               |
|------------------------------|---------|--------|-------------------------------------------|
| `audit_events_emitted_total` | Counter | --     | Audit events successfully emitted         |
| `audit_events_dropped_total` | Counter | --     | Audit events dropped due to a full buffer |

---

## auth_denylist_mirror

Package: `auth/denylist/mirror`

| Name                                   | Type    | Labels   | Description                     |
|----------------------------------------|---------|----------|---------------------------------|
| `auth_denylist_mirror_refreshes_total` | Counter | `result` | Snapshot refreshes              |
| `auth_denylist_mirror_snapshot_size`   | Gauge   | --       | Entries in the current snapshot |

---

## auth_denylist_negcache

Package: `auth/denylist/negcache`

| Name                                   | Type    | Labels             | Description            |
|----------------------------------------|---------|--------------------|------------------------|
| `auth_denylist_negcache_lookups_total` | Counter | `result`, `filter` | Negative-cache lookups |

---

## auth_oauth2client

Package: `auth/oauth2client`

| Name                                             | Type      | Labels            | Description          |
|--------------------------------------------------|-----------|-------------------|----------------------|
| `auth_oauth2client_token_fetches_total`          | Counter   | `grant`, `status` | Token fetches        |
| `auth_oauth2client_token_fetch_duration_seconds` | Histogram | `grant`, `status` | Token fetch duration |
| `auth_oauth2client_token_fetch_retries_total`    | Counter   | `grant`           | Token fetch retries  |

---

## auth_selfjwt

Package: `auth/selfjwt`

| Name                                                | Type      | Labels   | Description                    |
|-----------------------------------------------------|-----------|----------|--------------------------------|
| `auth_selfjwt_mints_total`                          | Counter   | `status` | Tokens minted                  |
| `auth_selfjwt_verifications_total`                  | Counter   | `status` | Token verifications            |
| `auth_selfjwt_mint_duration_seconds`                | Histogram | `status` | Mint duration                  |
| `auth_selfjwt_verify_duration_seconds`              | Histogram | `status` | Verification duration          |
| `auth_selfjwt_verification_key_cache_lookups_total` | Counter   | `result` | Verification-key cache lookups |

---

## auth_static

Package: `auth/static`

| Name                                      | Type      | Labels   | Description              |
|-------------------------------------------|-----------|----------|--------------------------|
| `auth_static_validations_total`           | Counter   | `status` | Token validations        |
| `auth_static_tokens_active`               | Gauge     | --       | Configured active tokens |
| `auth_static_validation_duration_seconds` | Histogram | --       | Validation duration      |

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

On subscriber metrics, `subject` is the subscription subject (which may contain wildcards), not the concrete subject of each message, so
cardinality stays bounded by the number of subscriptions. `broker_message_processing_errors_total` counts messages the handler rejected with
`Nak` / `Term` or that made the handler panic, once per delivery.

On publisher metrics, `subject` is the published subject; at most `DefaultSubjectLabelLimit` (256, configurable with
`broker.WithSubjectLabelLimit`) distinct subjects get their own series, and further subjects are counted under `_other`.

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
| `cache_negative_hits_total`       | Counter   | `cache_name` | Negative cache hits                  |
| `cache_errors_total`              | Counter   | `cache_name` | Cache operation errors               |
| `cache_write_duration_seconds`    | Histogram | --           | Cache write operation duration       |
| `cache_fallback_duration_seconds` | Histogram | --           | Fallback function execution duration |
| `cache_evictions_total`           | Counter   | `cache_name` | Cache evictions                      |
| `cache_size`                      | Gauge     | `cache_name` | Current cache entries                |

---

## dlock

Package: `data/locks/dlock`

| Name                             | Type      | Labels | Description                                            |
|----------------------------------|-----------|--------|--------------------------------------------------------|
| `dlock_locks_acquired_total`     | Counter   | --     | Locks successfully acquired                            |
| `dlock_locks_released_total`     | Counter   | --     | Locks successfully released (paired with acquired)     |
| `dlock_locks_failed_total`       | Counter   | --     | Lock acquisition failures                              |
| `dlock_acquire_duration_seconds` | Histogram | --     | Lock acquisition duration                              |
| `dlock_synchronizations_total`   | Counter   | --     | Synchronize calls completed                            |

---

## eventbus

Package: `domain/eventbus`

| Name                                | Type      | Labels            | Description                          |
|-------------------------------------|-----------|-------------------|--------------------------------------|
| `eventbus_publish_total`            | Counter   | `event`, `status` | Events published (via `NewObserved`) |
| `eventbus_publish_duration_seconds` | Histogram | `event`           | Publish duration                     |
| `eventbus_publish_no_handler_total` | Counter   | `event`           | Events published with no handler     |

---

## filter

Package: `data/filter`

| Name                            | Type      | Labels           | Description                           |
|---------------------------------|-----------|------------------|---------------------------------------|
| `filter_parse_duration_seconds` | Histogram | --               | CEL expression parse duration         |
| `filter_parse_errors_total`     | Counter   | --               | CEL expression parse failures         |

Only the parser is instrumented. The translators live in independent subpackages that are deliberately not given the collector, so there is no
per-backend translation counter.

---

## grpc

Package: `transport/grpc/interceptors/metrics`

The subsystem defaults to `DefaultMetricsSubsystem` (`grpc`) and can be changed through the interceptor options or config.

| Name                                         | Type      | Labels                | Description                                           |
|----------------------------------------------|-----------|-----------------------|-------------------------------------------------------|
| `grpc_server_requests_total`                 | Counter   | `method`, `status`    | Server requests                                       |
| `grpc_server_request_duration_seconds`       | Histogram | `method`, `status`    | Server request duration                               |
| `grpc_server_requests_in_flight`             | Gauge     | --                    | Requests in flight                                    |
| `grpc_server_requests_in_flight_by_method`   | Gauge     | `method`              | Requests in flight per method                         |
| `grpc_server_request_size_bytes`             | Histogram | `method`, `status`    | Request size (size metrics enabled)                   |
| `grpc_server_response_size_bytes`            | Histogram | `method`, `status`    | Response size (size metrics enabled)                  |
| `grpc_server_stream_messages_sent_total`     | Counter   | `method`, `status`    | Stream messages sent (stream metrics enabled)         |
| `grpc_server_stream_messages_received_total` | Counter   | `method`, `status`    | Stream messages received (stream metrics enabled)     |
| `grpc_server_stream_message_size_bytes`      | Histogram | `method`, `direction` | Stream message size (stream and size metrics enabled) |

---

## grpc_connection_pool

Package: `transport/grpc/client/pool`

| Name                                               | Type      | Labels             | Description                         |
|----------------------------------------------------|-----------|--------------------|-------------------------------------|
| `grpc_connection_pool_connections_created_total`   | Counter   | `target`           | Connections created                 |
| `grpc_connection_pool_connections_closed_total`    | Counter   | `target`, `reason` | Connections closed                  |
| `grpc_connection_pool_connections_reused_total`    | Counter   | `target`           | Connections reused from the pool    |
| `grpc_connection_pool_connection_errors_total`     | Counter   | `target`           | Connection creation failures        |
| `grpc_connection_pool_connections_active`          | Gauge     | --                 | Currently active connections        |
| `grpc_connection_pool_connections_in_use`          | Gauge     | --                 | Connections currently in use        |
| `grpc_connection_pool_connections_idle`            | Gauge     | --                 | Idle connections available          |
| `grpc_connection_pool_waiters`                     | Gauge     | --                 | Goroutines waiting for a connection |
| `grpc_connection_pool_connect_duration_seconds`    | Histogram | `target`           | Connection establishment duration   |
| `grpc_connection_pool_cleanup_duration_seconds`    | Histogram | --                 | Cleanup cycle duration              |
| `grpc_connection_pool_cleanup_connections_removed` | Counter   | --                 | Connections removed during cleanup  |

---

## health

Package: `observability/health`

| Name                                  | Type      | Labels | Description                        |
|---------------------------------------|-----------|--------|------------------------------------|
| `health_check_cycle_duration_seconds` | Histogram | --     | Health check cycle duration        |
| `health_status_changes_total`         | Counter   | --     | Health status changes detected     |
| `health_checks_performed_total`       | Counter   | --     | Individual health checks performed |

---

## http

Package: `transport/http/server/middlewares/metrics`

The subsystem defaults to `DefaultMetricsSubsystem` (`http`) and can be changed through the middleware options or config.

| Name                                   | Type      | Labels             | Description                          |
|----------------------------------------|-----------|--------------------|--------------------------------------|
| `http_server_requests_total`           | Counter   | `method`, `status` | Server requests                      |
| `http_server_request_duration_seconds` | Histogram | `method`, `status` | Server request duration              |
| `http_server_requests_in_flight`       | Gauge     | --                 | Requests in flight                   |
| `http_server_request_size_bytes`       | Histogram | `method`, `status` | Request size (size metrics enabled)  |
| `http_server_response_size_bytes`      | Histogram | `method`, `status` | Response size (size metrics enabled) |

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

| Name                                 | Type      | Labels             | Description                  |
|--------------------------------------|-----------|--------------------|------------------------------|
| `mongo_connect_duration_seconds`     | Histogram | --                 | MongoDB connect duration     |
| `mongo_ping_retries_total`           | Counter   | --                 | MongoDB ping retry attempts  |
| `mongo_transaction_duration_seconds` | Histogram | --                 | MongoDB transaction duration |
| `mongo_transaction_errors_total`     | Counter   | --                 | Failed MongoDB transactions  |
| `mongo_operations_total`             | Counter   | `op`, `collection` | MongoDB CRUD operations      |
| `mongo_operation_duration_seconds`   | Histogram | `op`, `collection` | MongoDB operation duration   |

---

## nats_kv_lease

Package: `data/internal/natskvlease`

| Name                             | Type    | Labels         | Description                            |
|----------------------------------|---------|----------------|----------------------------------------|
| `nats_kv_lease_operations_total` | Counter | `op`, `result` | Lease operations                       |
| `nats_kv_lease_lease_held`       | Gauge   | --             | Lease held status (1=held, 0=not held) |

---

## nats_leader_election

Package: `data/leadelect/providers/nats`

| Name                                                      | Type      | Labels         | Description                                  |
|-----------------------------------------------------------|-----------|----------------|----------------------------------------------|
| `nats_leader_election_lease_operations_total`             | Counter   | `op`, `result` | Lease operations                             |
| `nats_leader_election_is_leader`                          | Gauge     | --             | Current leader status (1=leader, 0=follower) |
| `nats_leader_election_camping_iteration_duration_seconds` | Histogram | --             | Camping loop iteration duration              |

---

## oidc

Package: `auth/oidc`

| Name                                      | Type      | Labels   | Description                                                           |
|-------------------------------------------|-----------|----------|-----------------------------------------------------------------------|
| `auth_oidc_token_validations_total`       | Counter   | `issuer` | Token validation attempts                                             |
| `auth_oidc_validation_errors_total`       | Counter   | `issuer` | Token validation errors                                               |
| `auth_oidc_revocation_check_errors_total` | Counter   | --       | Token revocation check failures                                       |
| `auth_oidc_validation_duration_seconds`   | Histogram | --       | Token validation duration                                             |
| `auth_oidc_cache_hits_total`              | Counter   | --       | Token cache hits                                                      |
| `auth_oidc_cache_misses_total`            | Counter   | --       | Token cache misses                                                    |
| `auth_oidc_jwks_refreshes_total`          | Counter   | --       | JWKS refresh operations                                               |
| `auth_oidc_jwks_refresh_errors_total`     | Counter   | --       | Failed JWKS refresh operations                                        |
| `auth_oidc_jwks_refresh_duration_seconds` | Histogram | --       | JWKS refresh duration                                                 |
| `auth_oidc_jwks_stale_rejections_total`   | Counter   | --       | Validations affected by an over-stale JWKS cache (enforced or warned) |

---

## opa

Package: `auth/opa`

| Name                                      | Type      | Labels   | Description                     |
|-------------------------------------------|-----------|----------|---------------------------------|
| `auth_opa_policy_reloads_total`           | Counter   | `result` | Policy reload operations        |
| `auth_opa_policy_reload_duration_seconds` | Histogram | --       | Policy reload duration          |
| `auth_opa_evaluations_total`              | Counter   | `result` | Policy evaluations              |
| `auth_opa_evaluation_duration_seconds`    | Histogram | --       | Policy evaluation duration      |
| `auth_opa_modules_loaded`                 | Gauge     | --       | Policy modules currently loaded |
| `auth_opa_decision_cache_lookups_total`   | Counter   | `result` | Decision cache lookups (`hit`/`miss`); emitted only when the decision cache is enabled |

---

## orderby

Package: `data/orderby`

| Name                             | Type      | Labels | Description             |
|----------------------------------|-----------|--------|-------------------------|
| `orderby_parse_duration_seconds` | Histogram | --     | order_by parse duration |
| `orderby_parse_errors_total`     | Counter   | --     | order_by parse errors   |

---

## outbox

Package: `data/outbox`

| Name                                            | Type      | Labels | Description                                        |
|-------------------------------------------------|-----------|--------|----------------------------------------------------|
| `outbox_events_dispatched_total`                | Counter   | --     | Events successfully dispatched                     |
| `outbox_events_dispatch_attempts_failed_total`  | Counter   | --     | Failed delivery attempts                           |
| `outbox_events_saved_total`                     | Counter   | --     | Events persisted to the outbox store               |
| `outbox_events_skipped_total`                   | Counter   | --     | Events skipped due to key compaction               |
| `outbox_events_rejected_total`                  | Counter   | --     | Events dead-lettered as permanently undeliverable  |
| `outbox_events_expired_total`                   | Counter   | --     | Events that expired before dispatch                |
| `outbox_events_retries_scheduled_total`         | Counter   | --     | Failures rescheduled for a later attempt           |
| `outbox_max_retries_exhausted_total`            | Counter   | --     | Events hitting the retry limit                     |
| `outbox_watch_notifications_total`              | Counter   | --     | Change notifications pushed to `Outbox.Watch`      |
| `outbox_events_in_flight`                       | Gauge     | --     | Events currently being dispatched                  |
| `outbox_events_pending`                         | Gauge     | --     | Backlog depth: events awaiting dispatch            |
| `outbox_events_in_progress`                     | Gauge     | --     | Events currently locked by a dispatcher            |
| `outbox_events_dead_lettered`                   | Gauge     | --     | Dead-letter depth: exhausted plus rejected events  |
| `outbox_events_oldest_pending_age_seconds`      | Gauge     | --     | Dispatch lag: age of the oldest undispatched event |
| `outbox_dispatch_cycle_duration_seconds`        | Histogram | --     | Dispatch cycle duration                            |
| `outbox_unlock_cycle_duration_seconds`          | Histogram | --     | Unlock cycle duration                              |
| `outbox_cleanup_cycle_duration_seconds`         | Histogram | --     | Cleanup cycle duration                             |
| `outbox_expire_cycle_duration_seconds`          | Histogram | --     | Expire cycle duration                              |
| `outbox_stats_cycle_duration_seconds`           | Histogram | --     | Stats cycle duration                               |

**Alert on the gauges, not the counters.** A stalled outbox and an idle one produce identical counter rates — zero — and differ only in
`outbox_events_pending` and `outbox_events_oldest_pending_age_seconds`. `outbox_events_dead_lettered` never decreases on its own: those events
are retained deliberately for inspection, so a non-zero value means an operator still has to look. Populating all four requires the stats task
to be scheduled (`statsSchedule`).

`outbox_watch_notifications_total` only moves when a watcher is running (`Outbox.Watch` against a store that supports change notifications). A
flat line while `outbox_events_saved_total` climbs means the watcher died or was never started and dispatch quietly fell back to the poll
interval — correct, but at the latency the schedule dictates.

**Renamed.** Two names no longer matched what they measured once the in-process retry loop was removed, and the backlog gauges did not share the
`outbox_events_` prefix of the rest of the package. Update dashboards and alert rules accordingly:

| Old                                     | New                                            |
|-----------------------------------------|------------------------------------------------|
| `outbox_events_dispatch_errors_total`   | `outbox_events_dispatch_attempts_failed_total` |
| `outbox_dispatch_retries_total`         | `outbox_events_retries_scheduled_total`        |
| `outbox_pending_events`                 | `outbox_events_pending`                        |
| `outbox_in_progress_events`             | `outbox_events_in_progress`                    |
| `outbox_dead_lettered_events`           | `outbox_events_dead_lettered`                  |
| `outbox_oldest_pending_event_age_seconds` | `outbox_events_oldest_pending_age_seconds`   |

`outbox_events_dispatch_attempts_failed_total` also counts differently: it used to increment once per event after all retries were spent, and now
increments once per failed attempt.

---

## probfilter

Package: `data/probfilter`

| Name                                  | Type      | Labels                  | Description                          |
|---------------------------------------|-----------|-------------------------|--------------------------------------|
| `probfilter_lookups_total`            | Counter   | `filter_name`, `result` | Probabilistic filter lookups (`result`: `positive`, `negative`, `error`) |
| `probfilter_adds_total`               | Counter   | `filter_name`           | Items added to probabilistic filters |
| `probfilter_lookup_duration_seconds`  | Histogram | `filter_name`           | Filter lookup duration               |
| `probfilter_rebuild_duration_seconds` | Histogram | --                      | Filter rebuild duration              |
| `probfilter_rebuild_errors_total`     | Counter   | --                      | Failed filter rebuild operations     |

A rebuild skipped because another process is rebuilding the shared Redis filter (`ErrRebuildInProgress`) records neither metric.

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

## saga

Package: `data/saga`

| Name                                  | Type      | Labels | Description                                       |
|---------------------------------------|-----------|--------|---------------------------------------------------|
| `saga_started_total`                  | Counter   | --     | Saga instances started                            |
| `saga_completed_total`                | Counter   | --     | Saga instances that committed all stages          |
| `saga_compensated_total`              | Counter   | --     | Saga instances that fully rolled back             |
| `saga_failed_total`                   | Counter   | --     | Saga instances that entered the Failed state      |
| `saga_in_flight`                      | Gauge     | --     | Saga instances currently executing                |
| `saga_steps_executed_total`           | Counter   | --     | Forward step actions that committed               |
| `saga_step_failures_total`            | Counter   | --     | Forward step actions that failed after retries    |
| `saga_step_retries_total`             | Counter   | --     | Step action retry attempts                        |
| `saga_compensations_total`            | Counter   | --     | Compensations that ran successfully               |
| `saga_compensation_failures_total`    | Counter   | --     | Compensations that failed after retries           |
| `saga_recovery_cycles_total`          | Counter   | --     | Background recovery cycles executed               |
| `saga_recovered_total`                | Counter   | --     | Stalled or timed-out instances picked up          |
| `saga_stage_duration_seconds`         | Histogram | --     | Forward stage execution duration                  |

Package: `data/saga/engines/jetstream`

| Name                                  | Type      | Labels | Description                                       |
|---------------------------------------|-----------|--------|---------------------------------------------------|
| `saga_engine_submitted_total`         | Counter   | --     | Start commands published to JetStream             |
| `saga_engine_consumed_total`          | Counter   | --     | Start commands received from JetStream            |
| `saga_engine_acked_total`             | Counter   | --     | Commands acked after a terminal saga state        |
| `saga_engine_naked_total`             | Counter   | --     | Commands returned for redelivery                  |
| `saga_engine_terminated_total`        | Counter   | --     | Poison commands terminated without redelivery     |
| `saga_engine_ack_errors_total`        | Counter   | --     | Failed Ack, Nak, Term or InProgress calls         |
| `saga_engine_in_flight`               | Gauge     | --     | Sagas the engine is currently driving             |

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

---

## plugins

Package: `plugins`

| Name                                              | Type      | Labels | Description                                           |
|---------------------------------------------------|-----------|--------|-------------------------------------------------------|
| `plugins_signature_verification_attempts_total`   | Counter   | --     | Plugin signature verification attempts                |
| `plugins_signature_verification_success_total`    | Counter   | --     | Successful plugin signature verifications             |
| `plugins_signature_verification_failures_total`   | Counter   | --     | Failed plugin signature verifications                 |
| `plugins_signature_verification_duration_seconds` | Histogram | --     | Plugin signature verification duration                |
| `plugins_load_attempts_total`                     | Counter   | --     | Plugin load attempts                                  |
| `plugins_load_success_total`                      | Counter   | --     | Successful plugin loads                               |
| `plugins_load_failures_total`                     | Counter   | --     | Failed plugin loads                                   |
| `plugins_load_duration_seconds`                   | Histogram | --     | Plugin load duration                                  |
| `plugins_loaded_total`                            | Gauge     | --     | Currently loaded plugins (a gauge despite the suffix) |
| `plugins_quarantine_added_total`                  | Counter   | --     | Plugins added to quarantine                           |
| `plugins_quarantine_cleared_total`                | Counter   | --     | Plugins cleared from quarantine                       |
| `plugins_quarantine_size`                         | Gauge     | --     | Currently quarantined plugins                         |
| `plugins_ready`                                   | Gauge     | --     | Plugins in ready state                                |
| `plugins_failed`                                  | Gauge     | --     | Plugins in failed state                               |
| `plugins_registered`                              | Gauge     | --     | Registered plugins                                    |
| `plugins_init_attempts_total`                     | Counter   | --     | Plugin Init calls                                     |
| `plugins_init_success_total`                      | Counter   | --     | Successful plugin Init calls                          |
| `plugins_init_failures_total`                     | Counter   | --     | Failed plugin Init calls                              |
| `plugins_init_duration_seconds`                   | Histogram | --     | Plugin Init duration                                  |
| `plugins_init_panics_total`                       | Counter   | --     | Panics during plugin Init                             |
