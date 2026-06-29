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
| `WithWatchChannelSize`   | 10        | Buffer size for policy event watch channels                  |
| `WithScheduler`          | nil       | Task registrar for periodic policy update cycles             |
| `WithUpdateSchedule`     | --        | Cron expression and run-on-start flag for scheduled updates  |
| `WithHealthCoordinator`  | nil       | Register manager with health coordinator                     |
| `WithAuditRecorder`      | nil       | Record every evaluation decision through an `*audit.Recorder` |

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
| `ErrSchedulerManaged`   | Direct call to a function managed by the scheduler     |

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
