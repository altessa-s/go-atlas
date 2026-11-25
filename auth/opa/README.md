# opa

```go
import "github.com/altessa-s/go-atlas/auth/opa"
```

Package `opa` provides Open Policy Agent (OPA) integration for authorization. Supports policy evaluation from various sources (embedded, filesystem,
GitLab, S3) with hot-reloading capabilities. Modular design with pluggable policy sources and event-driven architecture for policy updates.

## Key types

| Type / Interface | Description                                                                                              |
|------------------|----------------------------------------------------------------------------------------------------------|
| `Manager`        | Orchestrates policy loading, caching, evaluation, and hot-reload                                         |
| `Evaluator`      | Thread-safe interface for policy evaluation: `Evaluate`, `Query`                                         |
| `PolicySource`   | Passive policy fetcher: `Name`, `Fetch`, `Close` (the manager polls)                                     |
| `PolicyBundle`   | Collection of Rego modules with revision and optional data                                               |
| `Result`         | Evaluation outcome: `Allow` flag, optional `DecisionID`, and `Denials` (code → message; `HasDenialCode`) |
| `PolicyEvent`    | Event emitted on policy update or error, with source and revision info                                   |
| `WatchResult`    | Subscription handle with `Events` channel, `Stop`, and `Done`                                            |
| `WatchOptions`   | Watch configuration: buffer size, event types, overflow policy                                           |

## Options

| Option                  | Default | Description                                                 |
|-------------------------|---------|-------------------------------------------------------------|
| `WithLogger`            | discard | Structured logger (`*slog.Logger`)                          |
| `WithDecisionLogging`   | false   | Attach unique `DecisionID` to every evaluation result       |
| `WithWatchChannelSize`  | 10      | Watch event buffer when `WatchOptions.BufferSize` is zero   |
| `WithPollInterval`      | 30s     | Interval between source polls for policy changes            |
| `WithSchedulerTaskID`   | derived | ID used when registering the scheduled update task          |
| `WithScheduler`         | nil     | Task registrar for periodic policy update cycles            |
| `WithUpdateSchedule`    | --      | Cron expression and run-on-start flag for scheduled updates |
| `WithHealthCoordinator` | nil     | Register manager with a `health.Registrar`                  |
| `WithMeter`             | nil     | Metrics collector (subsystem `auth_opa`)                    |
| `WithAuditRecorder`     | nil     | Record every evaluation decision through `DecisionRecorder` |
| `WithDecisionCache`     | off     | Memoize evaluations by (query, policy revision, input) for a TTL   |

A nonempty schedule requires a registrar. `NewManager` returns `ErrSchedulerRequired` if no registrar is provided.

The default scheduler task ID is `opa-policy-update-` followed by the first 24 hex digits of the SHA-256 digest of the source's `Name()` and the Rego
query. Each input is prefixed with its byte length. An empty ID uses this default. Replicas with the same source name and query register the same task.
Managers with those same inputs sharing one registrar must set distinct IDs explicitly with `WithSchedulerTaskID` if each needs its own task.

## Decision cache

`WithDecisionCache(size, ttl)` short-circuits the Rego evaluation for a repeated input. It is off by default; the config equivalent is
`opa.cache.enabled` / `opa.cache.ttl` / `opa.cache.maxSize`. A non-positive size or TTL falls back to `DefaultDecisionCacheSize` (10000 entries) and
`DefaultDecisionCacheTTL` (5m).

Everything downstream of the decision still happens on a hit — the result is counted in `evaluations_total` and passed to the audit recorder. A
decision
that is served but never recorded is a hole in the audit trail, not an optimization.

Three properties make it safe to leave on:

- **Revision-keyed.** Entries are keyed by policy revision as well as input, so a bundle reload makes every prior decision *unreachable* rather than
  merely stale. A reload that lands while an evaluation is in flight prevents that result from being cached at all.
- **Fresh `DecisionID` per hit.** A `DecisionID` identifies one decision; replaying a cached one under its original ID would make distinct decisions
  indistinguishable in the audit log.
- **Independent copies.** `Result` has exported mutable fields, so each hit is handed its own copy — a caller enriching one result cannot corrupt the
  cached decision or any concurrent evaluation.

Use `WithDecisionCacheBackend(cache)` to supply a concurrency-safe cache. The caller controls its retention. Shared caches require revisions that
identify the same policy and data across sources.

Keys hash the query, revision and a length-prefixed encoding of the parsed Rego input, including the original bytes of strings. Queries that depend on
nondeterministic builtins, such as `time.now_ns()`, and inputs without lossless keys bypass the decision cache.

## Reload and shutdown

The built-in sources preserve JSON numbers and reject trailing tokens. Bundle revisions hash ordered, length-prefixed module and JSON entries.
`NewPolicyBundleWithData` returns an error if a value cannot be marshaled. Document paths use slash-separated, nonempty components. A child can extend
an object with a new property. Duplicate properties and children beneath scalars or arrays fail the whole reload. The active revision keeps serving on
failure, and a successful fetch of that revision clears the health error.

`NewManager` takes ownership of its source on success. On construction failure the caller owns cleanup. `StartWatching(ctx)` stops when its context is
canceled. `StartWatchingUntilClose` uses the manager lifetime. `Close` detaches scheduled callbacks and health checks, joins polling, reloads, and event
deliveries, then closes the source.

## Auditing

`WithAuditRecorder` wires a `DecisionRecorder` so every evaluation produces an authorization `Decision`. The recorder is nil-safe
and applies its own policies: under the default deny-only mode grants are dropped and only denials are recorded. Each decision carries `Action` set to
the manager's Rego query, an `engine=opa` attribute (plus `revision` when a bundle revision is loaded), and, for denials, a `Reason` of the result
`DecisionID` when decision logging is enabled or `"deny"` otherwise. Recording never changes the verdict in the default best-effort failure mode;
under
`audit.FailureRequired` a failed write on an *allowed* evaluation makes `Evaluate` return an error wrapping `audit.ErrAuditFailed`, so nothing
proceeds
unrecorded.

```go
manager, err := opa.NewManager(ctx, source, "data.authz.allow",
    opa.WithAuditRecorder(audit.NewRecorder(sink, audit.WithPolicyMode(audit.PolicyAll))),
)
```

## Errors

| Error                   | Description                                            |
|-------------------------|--------------------------------------------------------|
| `ErrSourceRequired`     | Policy source was not provided to NewManager           |
| `ErrQueryRequired`      | Rego query was not provided to NewManager              |
| `ErrPoliciesNotLoaded`  | Evaluate called before policies are loaded             |
| `ErrSourceClosed`       | Operation attempted on a closed policy source          |
| `ErrManagerClosed`      | Operation attempted on a closed manager                |
| `ErrNoPolicyFiles`      | No policy files found in the specified path            |
| `ErrBundleFetchFailed`  | Fetching the policy bundle from the source failed      |
| `ErrQueryPrepareFailed` | Compiling the Rego query against loaded modules failed |
| `ErrWatchStartFailed`   | Starting the policy watch on the source failed         |
| `ErrInvalidDataPath`    | A bundle data-path key is malformed                    |

A direct call to a scheduler-managed entry point returns `cycle.ErrManaged` from
[`go-atlas-core/runtime/cycle`](https://github.com/altessa-s/go-atlas-core/tree/develop/runtime/cycle).

## Subpackages

| Package                                    | Description                                            |
|--------------------------------------------|--------------------------------------------------------|
| [factory](../../factory/auth/opa)          | Config-based manager and evaluator creation            |
| [sources/embed](./sources/embed)           | Embedded fs.FS policy source (compile-time)            |
| [sources/filesystem](./sources/filesystem) | Filesystem policy source (reloaded by manager polling) |
| [sources/gitlab](./sources/gitlab)         | GitLab repository policy source via API                |
| [sources/s3](./sources/s3)                 | S3-compatible object store policy source               |

## Outbound HTTP

The factory always injects the resilient Atlas HTTP client for GitLab. For S3 it uses the Atlas client when `s3.proxy.mode: none` or `s3.ssrf` is set.
With both settings omitted, the AWS SDK handles HTTP transport and retries. Set `opa.gitlab.ssrf` or `opa.s3.ssrf` to configure egress policy for a
source. See the [Proxy guide](../../docs/proxy.md) and [factory README](../../factory/auth/opa/README.md#outbound-http-policy).
