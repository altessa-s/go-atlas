# factory

```go
import "github.com/altessa-s/go-atlas/transport/grpc/client/factory"
```

Maps configuration schemas to GRPC client options. Configuration stays independent of runtime clients.

## API

| Function | Behavior |
|----------|----------|
| `HealthOptions(cfg)` | Builds health-reporting options; nil yields no options. Validate the schema before assembly. |

The current gRPC mapping sets the service name. StateMapper and PerTarget require explicit client/pool options; this mapper does not configure them.

## Related packages

Proxy configuration is mapped by [`proxydial/factory`](../../../proxydial/factory/README.md). The corresponding [client](../README.md) owns runtime
behavior and its full programmatic option surface.
