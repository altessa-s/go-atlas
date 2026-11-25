# realip

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/realip"
```

Package `realip` provides middleware that extracts the real client IP address from request headers and stores it in the request context. Extraction is
delegated to a [`clientip.Extractor`](https://github.com/altessa-s/go-atlas-net/tree/develop/clientip) which handles trusted-proxy validation and header parsing (X-Forwarded-For by default,
opted-in single-address headers such as X-Real-IP). Downstream middleware and handlers retrieve the IP via `clientip.FromContext`. This middleware has no
dependencies and should be placed early in the chain so that other middleware (logger, limiter, tracing) can read the real IP from context.

## Usage

```go
extractor, err := clientip.NewExtractor(
    clientip.WithTrustedPeers(netip.MustParsePrefix("35.191.0.0/16")),
)
if err != nil {
    return err
}
handler := realip.Middleware(extractor, logger)(mux)
```

The factory uses a discard logger for extractor diagnostics by default. Enable both YAML `logClientDetails` and `logRawErrors` to send
extractor diagnostics to the server logger. For programmatic use, configure the supplied extractor logger separately. On address parse failures, middleware logs
the route template or `/_unknown`. `WithLogClientDetails(true)` adds the peer address. `WithLogRawErrors(true)` (YAML `logRawErrors: true`)
adds the underlying parse error, which can contain the address.
