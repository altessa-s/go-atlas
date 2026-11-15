# defaults

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/defaults"
```

Package `defaults` provides shared default values for gRPC interceptors. Centralizes common configuration to avoid duplication across interceptor
packages. Currently exposes `IgnorePatterns` which excludes gRPC reflection and health check service methods by default. The patterns are anchored:
they match only the methods of `grpc.health.v1.Health` and `grpc.reflection.v1[alpha].ServerReflection`, never a service whose name merely starts the
same way.

## Variables

| Variable          | Description                                                                          |
|-------------------|--------------------------------------------------------------------------------------|
| `IgnorePatterns`  | Default `[]*regexp.Regexp` that matches gRPC reflection and health check methods     |

The authentication ([`auth`](../auth)), recovery ([`recovery`](../recovery)) and guard ([`ipacl`](../ipacl), [`geoacl`](../geoacl),
[`limiter`](../limiter)) interceptors do not apply `IgnorePatterns` by default, so gRPC health and reflection are authenticated, recovered, access
controlled and rate-limited unless configured. Pass `auth.WithIgnorePatterns(defaults.IgnorePatterns...)` (or the same option of a guard) to opt in.
