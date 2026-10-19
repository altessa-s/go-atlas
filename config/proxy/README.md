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

## Credentials

`Config.URL` is a [`redacted.URL`](../../core/types/redacted): `fmt`, `log/slog`, JSON and YAML render it with the user-info password replaced by
`xxxxx` (an unparsable URL renders as `<redacted>`), and `Validate` errors never quote it or any part of it. `Validate` also rejects a URL whose
credentials spill into the host or path (more than one `@`, or an `@` outside the user info): percent-encode `@` and `/` in credentials. Factories read
the plain URL with `URL.Expose()`. `Auth.Password` is a `redacted.RedactedString` and is always rendered as `<redacted>`, which is why `mode: host` with
an `auth` block remains the preferred place for proxy secrets.

See the [config index](../README.md) for the other schema packages.
