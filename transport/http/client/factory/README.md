# factory

```go
import "github.com/altessa-s/go-atlas/transport/http/client/factory"
```

Maps configuration schemas to HTTP client options. Configuration stays independent of runtime clients.

## API

| Function | Behavior |
|----------|----------|
| `HealthOptions(cfg)` | Builds health-reporting options; nil yields no options. Validate the schema before assembly. |
| `SSRFOptions(cfg)` | Builds SSRF options; invalid CIDRs return an error, nil preserves client protection defaults. |

## Related packages

Proxy configuration is mapped by [`proxydial/factory`](../../../proxydial/factory/README.md). The corresponding [client](../README.md) owns runtime
behavior and its full programmatic option surface.
