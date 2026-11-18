# proxyconfig

```go
import proxyconfig "github.com/altessa-s/go-atlas/config/proxy"
```

Package `proxyconfig` defines the outbound HTTP proxy schema used by OIDC, OPA and tracing exporters. Schemas are populated by
[`go-nomos/loader`](https://github.com/altessa-s/go-nomos/tree/develop/loader) and consumed by the component factories, which map them to generated
options; runtime packages never import them.

## Modes

| Mode  | Effect |
|-------|--------|
| Empty | Keep the transport's configured proxy policy. |
| none  | Disable proxy resolution. |
| url   | Use an HTTP, HTTPS, SOCKS5 or SOCKS5H proxy URL with an explicit port. |
| host  | Use an HTTP proxy with a host, port and optional `auth.username` and `auth.password`. |

The fields come from `go-atlas-net/proxyconfig.Config`. URL mode accepts credentials, an empty or root path, and empty query and fragment components. Percent-encode `@` and `/` in
credentials. Host mode accepts resolver-specific names and scoped IPv6 addresses. The URL and password are redacted in diagnostic output. Validation
errors match `go-atlas-core/errors.ErrInvalidArgument`.

See the [config index](../README.md) for the other schema packages.
