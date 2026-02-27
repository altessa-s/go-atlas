# protovalidator

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/protovalidator"
```

Package `protovalidator` provides a gRPC server interceptor for validating incoming protocol buffer messages. Supports custom validation logic
via the `Validator` interface or `ValidatorFunc` adapter. Returns `codes.InvalidArgument` on validation failure. Validates streaming messages too.

## Key types

| Type / Interface | Description                                                                |
|------------------|----------------------------------------------------------------------------|
| `Validator`      | Interface: `Validate(ctx, proto.Message) error`                            |
| `ValidatorFunc`  | Function adapter for the `Validator` interface                             |

## Options

| Option               | Default            | Description                         |
|----------------------|--------------------|-------------------------------------|
| `WithIgnoreMethods`  | --                 | Methods to skip validation          |
| `WithIgnorePatterns` | reflection, health | Regex patterns for methods to skip  |
| `WithLogger`         | discard            | Structured logger                   |
