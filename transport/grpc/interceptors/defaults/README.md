# defaults

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/defaults"
```

Package `defaults` provides shared default values for gRPC interceptors. Centralizes common configuration to avoid duplication across interceptor
packages. Currently exposes `IgnorePatterns` which excludes gRPC reflection and health check service methods by default.

## Variables

| Variable          | Description                                                                          |
|-------------------|--------------------------------------------------------------------------------------|
| `IgnorePatterns`  | Default `[]*regexp.Regexp` that matches gRPC reflection and health check methods     |
