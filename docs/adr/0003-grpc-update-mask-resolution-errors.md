# ADR 0003. Report update mask resolution errors before resource body validation

Accepted on 2026-11-26.

## Context

For a unary Update RPC registered with `fieldmask.WithUpdateSpec`, the interceptor chain resolves the original mask and shares the result with
Buf and the handler. When resolution failed, Buf fell back to validating the whole resource. An empty required field could hide the indexed
mask error, even when the client intended to update a completely different field.

## Decision

When a registered mask cannot be resolved, Buf validates only the request fields outside the resource and the resource identity fields defined
by the registered spec. The handler reads `Selection.Err` and reports the indexed mask error before resource body validation. Buf omits resource
field rules and required-field checks beyond the identity scope. The field-behavior interceptor retains its existing format checks.

## Consequences

Clients can correct an invalid path without first supplying unrelated resource fields. Invalid request fields outside the resource or invalid
identity values can still fail validation before the handler runs. Resolved masks, unregistered methods, non-Update methods, and streams keep
their existing validation scope.
