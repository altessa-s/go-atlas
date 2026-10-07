# proxyconfig

```go
import proxyconfig "github.com/altessa-s/go-atlas/config/proxy"
```

Package `proxyconfig` defines the outbound HTTP proxy schema used by OIDC, OPA and tracing exporters. Schemas are populated by
[`config/loader`](../loader) and consumed by the component factories, which map them to generated options; runtime packages never import them.

## Key types

| Type        | Description                                                                                             |
|-------------|---------------------------------------------------------------------------------------------------------|
| `Mode` | Selects the proxy-resolution strategy for the transport/http/client and transport/grpc/client packages. |
| `Config`     | Is the shared outbound proxy configuration.                                                             |
| `Auth` | Carries proxy credentials.                                                                              |

See the [config index](../README.md) for the other schema packages.
