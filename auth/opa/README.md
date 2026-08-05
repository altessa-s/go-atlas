# opa

```go
import "github.com/altessa-s/go-atlas/auth/opa"
```

Package `opa` provides Open Policy Agent (OPA) integration for authorization. Supports policy evaluation from various sources (filesystem,
HTTP bundles) with hot-reloading capabilities. Modular design with pluggable policy sources and event-driven architecture for policy updates.

## Key types

| Type / Interface | Description                                                            |
|------------------|------------------------------------------------------------------------|
| `Manager`        | Orchestrates policy loading, caching, evaluation, and hot-reload       |
| `Evaluator`      | Thread-safe interface for policy evaluation: `Evaluate`, `Query`       |
| `PolicySource`   | Interface for fetching policies: `Name`, `Fetch`, `Watch`, `Close`     |
| `PolicyBundle`   | Collection of Rego modules with revision and optional data             |
| `Result`         | Evaluation outcome with `Allow` flag and optional `DecisionID`         |
| `PolicyEvent`    | Event emitted on policy update or error, with source and revision info |
| `WatchResult`    | Subscription handle with `Events` channel, `Stop`, and `Done`          |
| `WatchOptions`   | Watch configuration: buffer size, event types, overflow policy         |

## Options

| Option                   | Default   | Description                                                  |
|--------------------------|-----------|--------------------------------------------------------------|
| `WithLogger`             | discard   | Structured logger (`*slog.Logger`)                           |
| `WithDecisionLogging`    | false     | Attach unique `DecisionID` to every evaluation result        |
| `WithWatchChannelSize`   | 10        | Watch event buffer when `WatchOptions.BufferSize` is zero    |
| `WithScheduler`          | nil       | Task registrar for periodic policy update cycles             |
| `WithUpdateSchedule`     | --        | Cron expression and run-on-start flag for scheduled updates  |
| `WithHealthCoordinator`  | nil       | Register manager with health coordinator                     |
| `WithAuditRecorder`      | nil       | Record every evaluation decision through an `*audit.Recorder` |
| `WithDecisionCache`      | off       | Memoize evaluations by (policy revision, input) for a TTL    |

## Decision cache

`WithDecisionCache(size, ttl)` short-circuits the Rego evaluation for a repeated input. It is off by default; the config equivalent is
`opa.cache.enabled` / `opa.cache.ttl` / `opa.cache.maxSize`. A non-positive size or TTL falls back to `DefaultDecisionCacheSize` (10000 entries) and
`DefaultDecisionCacheTTL` (5m).

Everything downstream of the decision still happens on a hit — the result is counted in `evaluations_total` and passed to the audit recorder. A
decision that is served but never recorded is a hole in the audit trail, not an optimization.

Three properties make it safe to leave on:

- **Revision-keyed.** Entries are keyed by policy revision as well as input, so a bundle reload makes every prior decision *unreachable* rather than
  merely stale. A reload that lands while an evaluation is in flight prevents that result from being cached at all.
- **Fresh `DecisionID` per hit.** A `DecisionID` identifies one decision; replaying a cached one under its original ID would make distinct decisions
  indistinguishable in the audit log.
- **Independent copies.** `Result` has exported mutable fields, so each hit is handed its own copy — a caller enriching one result cannot corrupt the
  cached decision or any concurrent evaluation.

Keys are the SHA-256 of the revision and the JSON-encoded input; the input itself is never retained, since it typically carries the whole request.
Keying on JSON costs no generality, because OPA marshals the input to JSON to evaluate it in the first place.

Whether it pays off depends on policy complexity. On an Apple M4 Pro, a single-predicate policy evaluates in ~5.4 µs uncached against ~0.7 µs for a
cache hit; a realistic multi-field input raises the hit to ~1.8 µs, dominated by the encode-and-hash.

## Auditing

`WithAuditRecorder` wires an [`auth/audit`](../audit/) `*Recorder` so every evaluation produces an authorization `Decision`. The recorder is
nil-safe and applies its own policies: under the default deny-only mode grants are dropped and only denials are recorded. Each decision carries
`Action` set to the manager's Rego query, an `engine=opa` attribute (plus `revision` when a bundle revision is loaded), and, for denials, a
`Reason` of the result `DecisionID` when decision logging is enabled or `"deny"` otherwise. Recording never changes the verdict in the default
best-effort failure mode; under `audit.FailureRequired` a failed write on an *allowed* evaluation makes `Evaluate` return an error wrapping
`audit.ErrAuditFailed`, so nothing proceeds unrecorded.

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

A direct call to a scheduler-managed entry point returns `scheduler.ErrSchedulerManaged` from
[`core/scheduler`](../../core/scheduler/README.md).

## Subpackages

| Package                                    | Description                                  |
|--------------------------------------------|----------------------------------------------|
| [factory](./factory)                       | Config-based manager and evaluator creation  |
| [sources/embed](./sources/embed)           | Embedded fs.FS policy source (compile-time)  |
| [sources/filesystem](./sources/filesystem) | Filesystem policy source with fsnotify watch |
| [sources/gitlab](./sources/gitlab)         | GitLab repository policy source via API      |
| [sources/s3](./sources/s3)                 | S3-compatible object store policy source     |

## Outbound HTTP

The GitLab and S3 sources reach external services and accept proxy / retry /
breaker configuration through the resilient
[`transport/http/client`](../../transport/http/client/). GitLab forwards the
options via `gitlab.WithHTTPClientOptions(...)`; S3 swaps the AWS SDK
transport via `awsconfig.WithHTTPClient(httpclient.New(...))` only when
`s3.proxy` is explicitly configured (otherwise the SDK keeps its own
transport and retry layer). See the [Proxy guide](../../docs/proxy.md) for
YAML modes and the [factory README](./factory/) for wiring details.
