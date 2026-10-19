# errs

```go
import sagaerrs "github.com/altessa-s/go-atlas/data/saga/errs"
```

Sentinel errors returned by the saga orchestrator and its [`Storage`](../storage.go) backends. Match them with `errors.Is`; they are also returned
wrapped (via `core/errors`), so always use `errors.Is` rather than `==`. `ErrInstanceNotFound`, `ErrInstanceExists`, `ErrVersionConflict` and
`ErrInstanceBusy` wrap the canonical `core/errors` sentinels `ErrNotFound`, `ErrAlreadyExists`, `ErrConflict` and `ErrBusy`, so a transport
error mapper classifies them without knowing the saga package.

## Sentinels

| Error                   | Returned by                  | Meaning                                                            |
|-------------------------|------------------------------|-------------------------------------------------------------------|
| `ErrInstanceNotFound`   | `Storage.Get` / `Storage.Update` | No instance exists for the requested ID.                          |
| `ErrInstanceExists`     | `Storage.Create`             | An instance with the same ID already exists (makes Start idempotent). |
| `ErrVersionConflict`    | `Storage.Update`             | Optimistic-concurrency CAS lost; another coordinator advanced it (benign). |
| `ErrDefinitionNotFound` | `Orchestrator.Resume`        | The persisted definition name does not match the orchestrator.    |
| `ErrAlreadyTerminal`    | `Orchestrator.Resume`        | The instance is already in a terminal status; nothing left to run. |
| `ErrCompensationFailed` | `Orchestrator`               | A compensation exhausted its retries; the saga moved to `Failed`. |
| `ErrNoCompensation`     | `Builder.Build`              | `PolicyEnforce` set and a compensatable step lacks a compensation. |
| `ErrEmptyDefinition`    | `Builder.Build`              | No steps were added to the builder.                               |
| `ErrEmptyID`            | `Orchestrator.Start` / `Resume` | The instance ID is empty.                                      |
| `ErrInstanceBusy`       | `Orchestrator.Start` / `Resume` | Another invocation holds the execution lease.                  |

## See also

- [data/saga](../README.md) — the orchestrator that returns these errors.
