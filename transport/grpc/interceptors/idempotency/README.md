# idempotency

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/idempotency"
```

Package `idempotency` provides a gRPC server interceptor for idempotent request handling. Prevents duplicate processing using an idempotency key
from gRPC metadata (default header: `Idempotency-Key`). Uses the driven interceptor pattern. Duplicate requests receive the stored response.

## Key types

| Type / Interface     | Description                                                                |
|----------------------|----------------------------------------------------------------------------|
| `ErrorScenario`      | Identifies the reason for an idempotency error (Missing, Invalid, etc.)   |
| `KeyFormatValidator` | Validates idempotency key format (default: UUID v4)                        |
| `EntityIdExtractor`  | Extracts entity ID from successful response for status metadata            |
| `StatusCreator`      | Creates custom gRPC status errors for idempotency scenarios                |

## Options

| Option                            | Default                    | Description                                 |
|-----------------------------------|----------------------------|---------------------------------------------|
| `WithIdempotencyKeyHeader`        | `Idempotency-Key`          | Metadata header name for the key            |
| `WithIdempotencyKeyStatusMetadata`| `Idempotency-Key-Status`   | Response metadata key for status            |
| `WithIdempotencyKeyEntityIdMetadata`| `Idempotency-Key-Entity-Id`| Response metadata key for entity ID        |
| `WithFallbackBehavior`            | Deny                       | Behavior on storage failure (Allow/Deny/Error) |
| `WithEnforceMandatory`            | false                      | When true, require idempotency key on all requests |
| `WithKeyFormatValidator`          | UUID v4                    | Custom key format validation function        |
| `WithEntityIdExtractor`           | nil                        | Extract entity ID from response              |
| `WithStatusCreator`               | default messages           | Custom error status creation function        |
| `WithIgnoreMethods`               | --                         | Methods to skip idempotency checking         |
| `WithIgnorePatterns`              | reflection, health         | Regex patterns for methods to skip           |
