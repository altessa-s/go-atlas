# le

```go
import "github.com/altessa-s/go-atlas/security/tlsutils/providers/le"
```

Package `le` provides a Let's Encrypt TLS certificate provider using HTTP-01 ACME challenges. Wraps
`autocert` for automatic certificate management with renewal and caching.

## Usage

```go
p := le.New(
    le.WithDomains("example.com", "www.example.com"),
    le.WithCacheDir("/var/lib/acme"),
    le.WithEmail("admin@example.com"),
)
defer p.Close(ctx)

tlsCfg := p.TLSConfig()
p.StartHTTPServer(":80", nil) // HTTP-01 challenge server
```
