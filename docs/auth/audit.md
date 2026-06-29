# Authorization-Decision Audit

A durable, structured record of every authentication and authorization decision — who was allowed or denied which action on which resource,
and why. Distinct from aggregate metrics and ephemeral logs: this is the access-control trail compliance regimes (SOC 2, ISO 27001, PCI,
HIPAA) require and the forensic record for "who could touch resource X, and when".

---

## Table of Contents

- [Overview](#overview)
- [Why a separate signal](#why-a-separate-signal)
- [Model](#model)
- [Policies](#policies)
- [Quick Start](#quick-start)
  - [Recorder and sink](#recorder-and-sink)
  - [scope (gRPC / HTTP)](#scope-grpc--http)
  - [static (gRPC / HTTP)](#static-grpc--http)
  - [oidc (gRPC)](#oidc-grpc)
  - [opa (core)](#opa-core)
  - [data/audit bridge](#dataaudit-bridge)
- [API Reference](#api-reference)
- [Design Notes](#design-notes)
- [See Also](#see-also)

## Overview

The `auth/audit` package is a pure primitive: a `Decision` value object, a consumer-side `Sink` seam, and a `Recorder` that applies recording
and failure policies around the sink. It performs no I/O and depends on no storage. The deciding engines (`auth/scope`, `auth/opa`) stay pure
and unaware of auditing; a transport adapter — or the application — builds a `Decision` from the request context (where timing, trace id, and
peer live) and hands it to a `Recorder`. Persistence is whatever `Sink` the caller supplies: the project's `data/audit` dispatcher, a log
stream, any store.

```go
import "github.com/altessa-s/go-atlas/auth/audit"
```

## Why a separate signal

|            | metrics                  | logs                  | **audit**                                                      |
|------------|--------------------------|-----------------------|---------------------------------------------------------------|
| purpose    | aggregates (rates)       | debugging             | per-decision: who, what, on which resource, allowed, why      |
| retention  | time series              | ephemeral / rotated   | durable, queryable, retained for compliance                   |
| answers    | "how many denials/sec"   | unstructured trace    | "who accessed resource X on March 12, and was it allowed"     |

A metric says "5 denials happened"; a log says it unstructured and is later rotated away. Audit is the immutable structured record of each
decision, fit for investigation and regulator review.

## Model

| Concept       | Type                                                                                              | Role                                                                                          |
|---------------|--------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------|
| Decision      | `audit.Decision`                                                                                  | One outcome: `Time`, `Allowed`, `Subject`, `Action`, `ResourceType`/`ResourceID`, `RequiredScope`, `Reason`, `Attributes`. |
| Sink          | `audit.Sink` — `Record(ctx, Decision) error`                                                      | The sole consumer-side seam that persists a decision. `SinkFunc` adapts a function.            |
| Recorder      | `*audit.Recorder`                                                                                 | Applies the policies around a `Sink`. Nil `*Recorder` and nil sink are valid no-ops.           |
| PolicyMode    | `audit.PolicyDenyOnly` (default) / `audit.PolicyAll`                                              | Whether grants are recorded or only denials.                                                   |
| FailureMode   | `audit.FailureBestEffort` (default) / `audit.FailureRequired`                                     | What a sink write error means.                                                                 |

`Decision.Time` is caller-supplied — the package keeps no clock. Free-form fields (`Reason`, `Attributes`) should carry stable,
low-cardinality tokens, and no field may hold raw credentials.

## Policies

`PolicyMode` bounds volume. `PolicyDenyOnly` (default) captures the security-relevant denials without a write per authorized request;
`PolicyAll` keeps a full access trail.

`FailureMode` decides what a sink write error means. `FailureBestEffort` (default) discards it — auditing must never become a
denial-of-service vector. `FailureRequired` returns it wrapping `ErrAuditFailed` ("no audit, no action"); the returned error is the caller's
signal to fail the request because it could not be recorded — it is **not** an authorization verdict. Adapters honor this only on an
otherwise-successful decision (a request already failing on a denial is not failed twice).

## Quick Start

### Recorder and sink

```go
sink := dataaudit.New(auditor) // any audit.Sink; see the bridge below
rec := audit.NewRecorder(sink,
    audit.WithPolicyMode(audit.PolicyAll),        // record grants too, not only denials
    audit.WithFailureMode(audit.FailureRequired)) // fail the request if it can't be recorded
```

A nil sink yields a no-op `Recorder`, so auditing can be wired unconditionally and switched on by configuration.

### scope (gRPC / HTTP)

Both scope adapters take a non-breaking variadic `WithScopeAudit[P]`. After the verdict they record a `Decision` keyed on the action
(`Action` = gRPC full method or HTTP route), with `Reason` `scope_denied` or `principal_type_mismatch` and `Attributes{"transport": …}`.

```go
import grpcauth "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"

interceptor := grpcauth.ServerInterceptor(
    grpcauth.WithAuthFunc(authFunc),
    grpcauth.WithClientAuth(grpcauth.ScopeClientAuth(enf,
        grpcauth.WithScopeAudit(rec, func(p *Principal) string { return p.Subject }))),
)
```

```go
import httpauth "github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth"

mw := httpauth.ScopeMiddleware(enf, keyFunc,
    httpauth.WithScopeAudit(rec, func(p *Principal) string { return p.Subject }))
```

### static (gRPC / HTTP)

The static-token adapters record an authentication decision (`Action` = `"authenticate"`). `WithAudit` takes the recorder and a
`subjectOf func(any) string` that extracts the principal identity from the value the store returned on success. `Reason` is one of
`invalid_token`, `empty_token`, `rate_limited`, `missing_token`, or `error`.

```go
import grpcstatic "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth/static"

authFunc := grpcstatic.AuthFunc(store,
    grpcstatic.WithAudit(rec, func(data any) string { return data.(*User).ID }))
```

### oidc (gRPC)

The OIDC interceptor records token validation (`Action` = `"validate_token"`). `subjectOf` receives the `*Claims` returned by the validator;
`Reason` maps the validation error to `revoked`, `invalid_token`, `missing_token`, or `error`.

```go
import grpcoidc "github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth/oidc"

authFunc := grpcoidc.AuthFunc(validator,
    grpcoidc.WithAudit(rec, func(c *grpcoidc.Claims) string { return c.Subject }))
```

### opa (core)

OPA has no transport adapter, so auditing is a `Manager` option. Each `Evaluate` records a `Decision` (`Action` = the manager's query,
`Reason` = the decision id or `deny`, `Attributes{"engine": "opa"}`). Under `FailureRequired`, a record error on an allowed evaluation
fails the evaluation.

```go
mgr, err := opa.NewManager(ctx, source, query, opa.WithAuditRecorder(rec))
```

### data/audit bridge

`auth/audit/sinks/dataaudit` adapts the framework's `data/audit` dispatcher to an `audit.Sink`, mapping each `Decision` to a `data/audit`
`Event`. This is the usual durable sink in production; the `auth/audit` core stays free of that dependency.

```go
import "github.com/altessa-s/go-atlas/auth/audit/sinks/dataaudit"

sink := dataaudit.New(auditor) // auditor must be Started; implements audit.Sink
```

## API Reference

| Symbol                                       | Description                                                                                          |
|----------------------------------------------|-----------------------------------------------------------------------------------------------------|
| `audit.Decision`                             | One authorization/authentication outcome (see [Model](#model)).                                       |
| `audit.Sink` / `audit.SinkFunc`              | `Record(ctx, Decision) error` seam and its function adapter.                                          |
| `audit.NewRecorder(sink, opt…)`              | Recorder writing to `sink`; nil sink yields a no-op.                                                  |
| `Recorder.Record(ctx, d)`                    | Submit a decision under the configured policies; non-nil only under `FailureRequired`.                |
| `audit.WithPolicyMode(m)`                    | `PolicyDenyOnly` (default) / `PolicyAll`.                                                             |
| `audit.WithFailureMode(m)`                   | `FailureBestEffort` (default) / `FailureRequired`.                                                    |
| `audit.ErrAuditFailed`                       | Wrapped by `Record` under `FailureRequired` when the sink errors.                                     |
| `dataaudit.New(auditor)`                     | `audit.Sink` bridging to the `data/audit` dispatcher.                                                 |
| `grpcauth.WithScopeAudit(rec, subjectOf)`    | Audit option for the gRPC scope adapter (authz; action = full method).                                |
| `httpauth.WithScopeAudit(rec, subjectOf)`    | Audit option for the HTTP scope middleware (authz; action = route key).                               |
| `grpcstatic.WithAudit(rec, subjectOf)`       | Audit option for the gRPC static adapter (authn; action = `authenticate`).                            |
| `httpstatic.WithAudit(rec, subjectOf)`       | Audit option for the HTTP static adapter (authn; action = `authenticate`).                            |
| `grpcoidc.WithAudit(rec, subjectOf)`         | Audit option for the gRPC OIDC adapter (authn; action = `validate_token`).                            |
| `opa.WithAuditRecorder(rec)`                 | Audit option for the OPA `Manager` (authz; action = query, recorded in `Evaluate`).                   |

## Design Notes

- **Primitive first, adapters later.** The core ships only `Decision` / `Sink` / `Recorder`; the `data/audit` bridge and the transport wiring
  are layered separately so the deciding engines never depend on storage.
- **Consumer-side seam.** `Sink` is the single extension point and lives with the consumer; no field of `Decision` is dictated by any backend.
- **Non-breaking opt-in.** Every adapter audit hook is a variadic option; existing callers compile and behave unchanged, and a nil recorder is
  a zero-overhead no-op.
- **Authn vs authz.** `scope` and `opa` record authorization (`Action` = the protected key/query); `static` and `oidc` record authentication
  (`Action` = `authenticate` / `validate_token`). The split keeps `Reason` tokens meaningful per layer.

## See Also

- [`auth/audit` README](../../auth/audit/README.md) — package quick reference.
- [scope.md](scope.md) — the authorization policy whose decisions this trail records.
- [opa.md](opa.md) — OPA policy evaluation with the `WithAuditRecorder` option.
- [../metrics.md](../metrics.md) — the aggregate signal that complements this per-decision trail.
- [architecture.md](../architecture.md) — package map and layering.
