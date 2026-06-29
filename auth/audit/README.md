# audit

```go
import "github.com/altessa-s/go-atlas/auth/audit"
```

Package `audit` records **authorization decisions** — who was allowed or denied which action on which resource, and why — as durable,
structured, queryable events, distinct from aggregate metrics and ephemeral logs. It is the access-control audit trail compliance
regimes (SOC 2, ISO 27001, PCI, HIPAA) require, and the forensic record for "who could touch resource X, and when".

The deciding engines (`auth/scope`, `auth/opa`) stay pure and unaware of auditing. A transport adapter or the application builds a
`Decision` from the request context — where timing, trace id, and peer live — and hands it to a `Recorder`. Persistence is a
caller-supplied `Sink` (the project's `data/audit` dispatcher, a log stream, any store); this package ships no sink and performs no I/O
of its own.

## Key types

| Type          | Description                                                                                           |
|---------------|-----------------------------------------------------------------------------------------------------|
| `Decision`    | One authorization outcome: time, allowed, subject, action, optional resource, required scope, reason, attributes. |
| `Sink`        | `Record(ctx, Decision) error` — the sole consumer-side seam that persists a decision.                |
| `SinkFunc`    | Function adapter for `Sink`.                                                                         |
| `Recorder`    | Applies the recording and failure policies around a `Sink`. Nil `*Recorder` / nil sink are no-ops.   |
| `PolicyMode`  | `PolicyDenyOnly` (default) records only denials; `PolicyAll` records grants too.                     |
| `FailureMode` | `FailureBestEffort` (default) discards sink errors; `FailureRequired` propagates them.               |

## Functions

| Function                       | Description                                                                                    |
|--------------------------------|------------------------------------------------------------------------------------------------|
| `NewRecorder(sink, opt…)`      | Recorder writing to `sink`; nil sink yields a no-op so auditing can be wired unconditionally.   |
| `Recorder.Record(ctx, d)`      | Submit a decision under the configured policies; returns non-nil only under `FailureRequired`.   |
| `WithPolicyMode(m)`            | Select `PolicyDenyOnly` / `PolicyAll`.                                                          |
| `WithFailureMode(m)`           | Select `FailureBestEffort` / `FailureRequired`.                                                 |

## Policies

`PolicyMode` bounds volume: `PolicyDenyOnly` (default) captures the security-relevant denials without a write per authorized request;
`PolicyAll` keeps a full access trail.

`FailureMode` decides what a sink write error means. `FailureBestEffort` (default) discards it — auditing must never become a
denial-of-service vector. `FailureRequired` returns it wrapping `ErrAuditFailed` ("no audit, no action"); the returned error is the
caller's signal to fail the request because it could not be recorded, not an authorization verdict.

## Usage

```go
rec := audit.NewRecorder(sink, audit.WithPolicyMode(audit.PolicyAll))

// In a transport auth adapter, after the scope/opa decision:
err := enf.Enforce(principal, key)
_ = rec.Record(ctx, audit.Decision{
    Time:          now,
    Allowed:       err == nil,
    Subject:       principal.Subject,
    Action:        key,
    RequiredScope: required,
    Reason:        reasonOf(err), // e.g. "scope_denied", "principal_type_mismatch"
    Attributes:    map[string]string{"trace_id": traceID, "tenant": tenant},
})
```

`Decision.Time` is caller-supplied (the package keeps no clock); free-form fields should carry stable, low-cardinality tokens, and no
field may hold raw credentials.

## Scope

This is a primitive only. A `Sink` bridging to `data/audit` and the wiring into the gRPC/HTTP auth adapters (mapping `scope` /
`opa` outcomes — including `scope`'s principal-type-mismatch versus genuine denial — into a `Decision`) are layered separately.
