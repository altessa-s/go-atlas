# buf

```go
import bufhelpers "github.com/altessa-s/go-atlas/transport/grpc/interceptors/protovalidator/buf"
```

Package `bufhelpers` provides helpers for working with buf protovalidate validation errors in gRPC interceptors.
It converts validation violations into structured error responses with detailed field-level information.

Each violation's `FieldViolation.Code` is an optional **service-defined, client-facing reason code**, set only
when a [`ReasonCoder`](#) is supplied via `WithReasonCode`. go-atlas ships **no** built-in mapping — the set of
reason codes is part of a service's public error contract, so the service owns it. When no coder is supplied the
`Code` is left unset and downstream consumers fall back to the generic gRPC-status reason.

## Usage

```go
// Create a validator. Pass WithReasonCode to map a rule ID + field name to a
// service-defined reason code; without it, no code is emitted.
validator := bufhelpers.BuildValidator(
    bufhelpers.BuildValidationFilter(),
    bufhelpers.WithReasonCode(func(ruleID, field string) string {
        if ruleID == "required" {
            return strcase.ToScreamingSnake(field) + "_REQUIRED"
        }
        return "" // fall back to the gRPC-status reason
    }),
)

// Validate a proto message
if err := validator(ctx, msg); err != nil {
    // Error carries a BadRequest detail; FieldViolations[].Code holds the
    // service's reason code when a ReasonCoder is configured.
    return err
}
```

## Key Functions

| Function | Description |
|----------|-------------|
| `BuildValidator` | Creates a proto message validator with structured error details; accepts `WithReasonCode` |
| `WithReasonCode` | Supplies a `ReasonCoder` (`func(ruleID, fieldName string) string`) for field violation codes |
| `BuildValidationFilter` | Returns filter controlling which messages are validated |
| `BuildValidationError` | Formats violations into human-readable error strings |

## Features

- Lazy singleton validator initialization
- Structured `BadRequest` error details for gRPC
- Field path support with map keys and array indices
- Human-readable error formatting
- Optional service-defined reason codes on field violations
- Integration with buf's protovalidate library