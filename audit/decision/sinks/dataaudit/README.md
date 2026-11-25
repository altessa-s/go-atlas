# dataaudit

```go
import "github.com/altessa-s/go-atlas/audit/decision/sinks/dataaudit"
```

Package `dataaudit` bridges the authorization-decision `audit.Sink` seam to the project's general-purpose [`audit`](../../../../audit)
dispatcher. It is the wiring that keeps the `audit/decision` primitive free of any dependency on `audit`: the core declares the seam, this adapter
satisfies it.

## Mapping

A `Sink` maps each `audit.Decision` to a `audit` `Event` of type `EventTypeAuth`, action `ActionExecute`:

| `audit.Decision`               | `audit` Event                                  |
|--------------------------------|------------------------------------------------|
| `Subject`                      | `Actor.ID`                                     |
| `Action`                       | `Resource.Path`                                |
| `ResourceType` / `ResourceID`  | `Resource.Type` / `Resource.ID`                |
| `Allowed`                      | `Result.Status` = `success` / `denied`         |
| `Reason` (denials)             | `Result.Message`                               |
| `RequiredScope` + `Attributes` | `Metadata` (`required_scope` + each attribute) |
| `Time`                         | `Event.Timestamp` (auto-filled if zero)        |

A dropped event (full buffer) surfaces as `ErrDropped`, so an `audit.Recorder` under `FailureRequired` can fail the request.

## Functions

| Function             | Description                                                                    |
|----------------------|--------------------------------------------------------------------------------|
| `New(auditor)`       | Emit through an `Emitter`, such as a started `*audit.Auditor`. A nil emitter is a no-op. |
| `Sink.Record(ctx,d)` | Map and emit `d`; returns `ErrDropped` if the auditor dropped the event.       |

## Usage

```go
auditor, _ := dataudit.New(dispatcher) // audit; dispatcher already started
_ = auditor.Start()

rec := audit.NewRecorder(dataaudit.New(auditor)) // audit.Recorder over the bridge
// ... in a transport auth adapter, after the scope/opa decision:
_ = rec.Record(ctx, decision)
```
