# requestid

```go
import "github.com/altessa-s/go-atlas/transport/requestid"
```

Package `requestid` reads UUID v4 request IDs from HTTP headers and gRPC metadata. A generator can supply a new ID when the incoming value is missing
or invalid.

## Key types

| Symbol                       | Purpose                                              |
|------------------------------|------------------------------------------------------|
| `Generator.Extract`          | Validates the header and optionally generates an ID  |
| `NewContext` / `FromContext` | Stores and retrieves a request ID in request context |
