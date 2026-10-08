# realip

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/realip"
```

Package `realip` provides middleware that extracts the real client IP address from request headers and stores it in the request context. Extraction is
delegated to a [`clientip.Extractor`](../../../../clientip) which handles trusted-proxy validation and header parsing (X-Forwarded-For, X-Real-IP,
etc.). Downstream middleware and handlers retrieve the IP via `FromContext`. This middleware has no dependencies and should be placed early in the chain
so that other middleware (logger, limiter, tracing) can read the real IP from context.

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
