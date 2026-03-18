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
| [sources/s3](./sources/s3)                 | S3-compatible object store policy source     |
