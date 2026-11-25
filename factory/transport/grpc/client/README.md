# factory

```go
import "github.com/altessa-s/go-atlas/factory/transport/grpc/client"
```

Maps configuration schemas to gRPC client options. Configuration stays independent of runtime clients.

## API

| Function | Behavior |
|----------|----------|
| `HealthOptions(cfg)` | Builds health-reporting options; nil yields no options. Validate the schema before assembly. |
| `ConnectionOptions(cfg)` | Maps [`grpcclientconfig.Config`](../../../../config/grpcclient) to `WithConnectBackoff` and, when `resolver.enabled` is true, `WithDNSResolver`. The default preserves grpc-go's resolver selection, including DNS TXT service configs and globally registered builders. Validate the schema before assembly. |

The health mapping sets the service name. StateMapper and PerTarget require explicit client/pool options; this mapper does not configure them.

## Related packages

Proxy configuration is mapped by [`factory/ext/net/proxydial/README.md`](../../../ext/net/proxydial/README.md). The corresponding
[client](../../../../transport/grpc/client/README.md) owns runtime behavior and its full programmatic option surface.
