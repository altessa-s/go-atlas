# buf

```go
import bufhelpers "github.com/altessa-s/go-atlas/transport/grpc/interceptors/protovalidator/buf"
```

Package `bufhelpers` provides helpers for working with buf protovalidate validation errors in gRPC interceptors.
It converts validation violations into structured error responses with detailed field-level information.

Each violation's `FieldViolation.Code` is a **canonical, client-facing reason code** — never the raw
protovalidate rule ID. Standard rules map to registry codes (`int64.gte` → `INVALID_MIN_LENGTH_OR_VALUE`,
`string.email` → `INVALID_FORMAT_EMAIL`), `required` becomes `{FIELD}_REQUIRED`, and any rule that is
neither a known standard rule nor present in the supplied catalog resolves to `UNKNOWN`. See the
[`reasoncode`](../reasoncode) package for the mapping and `WithResolver` to register a service's own
rule catalog.

## Usage

```go
// Create a validator. Pass WithResolver to map a service's own rule IDs to
// canonical reason codes; standard rules are mapped out of the box.
validator := bufhelpers.BuildValidator(
    bufhelpers.BuildValidationFilter(),
    bufhelpers.WithResolver(reasoncode.NewResolver(myService.ReasonCodeCatalog)),
)

// Validate a proto message
if err := validator(ctx, msg); err != nil {
    // Error carries a BadRequest detail whose FieldViolations[].Code are
    // canonical reason codes (e.g. INVALID_MIN_LENGTH_OR_VALUE).
    return err
}
```

## Key Functions

| Function | Description |
|----------|-------------|
| `BuildValidator` | Creates a proto message validator with structured error details; accepts `WithResolver` |
| `BuildErrorCode` | Maps a rule ID + field path to a canonical reason code (never the raw rule ID) |
| `WithResolver` | Registers a service's reason-code catalog for the validator |
| `BuildValidationFilter` | Returns filter controlling which messages are validated |
| `BuildValidationError` | Formats violations into human-readable error strings |

## Features

- Lazy singleton validator initialization
- Structured `BadRequest` error details for gRPC
- Field path support with map keys and array indices
- Human-readable error formatting
- Automatic error code generation from field violations
- Integration with buf's protovalidate library