# buf

```go
import bufhelpers "github.com/altessa-s/go-atlas/transport/grpc/interceptors/protovalidator/buf"
```

Package `bufhelpers` provides helpers for working with buf protovalidate validation errors in gRPC interceptors.
It converts validation violations into structured error responses with detailed field-level information.

## Usage

```go
// Create validator function
validator := bufhelpers.BuildValidator()

// Validate a proto message
if err := validator(msg); err != nil {
    // Error contains structured BadRequest details
    return err
}

// Build validation filter
filter := bufhelpers.BuildValidationFilter()

// Generate error codes from violations
code := bufhelpers.BuildErrorCode("required", "userName")
// Returns: "USER_NAME_REQUIRED"
```

## Key Functions

| Function | Description |
|----------|-------------|
| `BuildValidator` | Creates a proto message validator with structured error details |
| `BuildErrorCode` | Derives error codes from rule IDs and field paths |
| `BuildValidationFilter` | Returns filter controlling which messages are validated |
| `BuildValidationError` | Formats violations into human-readable error strings |

## Features

- Lazy singleton validator initialization
- Structured `BadRequest` error details for gRPC
- Field path support with map keys and array indices
- Human-readable error formatting
- Automatic error code generation from field violations
- Integration with buf's protovalidate library