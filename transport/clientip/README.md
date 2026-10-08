# clientip

```go
import "github.com/altessa-s/go-atlas/transport/clientip"
```

Package `clientip` resolves the real client IP of a request that arrived through reverse proxies. `Extractor` walks the configured proxy
headers right to left, skipping trusted proxies and private addresses, and returns the first address outside the trusted infrastructure; with
no usable header it returns the peer address. It is transport-agnostic: the HTTP [`realip`](../http/server/middlewares/realip) middleware and
the gRPC [`realip`](../grpc/interceptors/realip) interceptor both use it, and code outside the middleware chain can call it directly.

## Options

| Option                    | Default                   | Description                                                        |
|---------------------------|---------------------------|--------------------------------------------------------------------|
| `WithTrustedPeers`        | none                      | Public peer prefixes whose proxy headers are honored (private peers always are) |
| `WithTrustedProxies`      | none                      | Prefixes skipped as proxy hops inside the forwarded headers        |
| `WithTrustedProxiesCount` | `0`                       | Number of trusted hops at the right end of `X-Forwarded-For`       |
| `WithHeaders`             | `DefaultHeaders()`        | Ordered proxy headers to consult                                   |
| `WithHeadersEnabled`      | `true`                    | Consult proxy headers at all                                       |
| `WithCacheSize`           | `1000`                    | LRU size for parsed addresses                                      |
| `WithCacheDisabled`       | `false`                   | Disable the parse cache                                            |
| `WithLogger`              | discard                   | Structured logger                                                  |

## Key types

| Symbol                         | Description                                                             |
|--------------------------------|-------------------------------------------------------------------------|
| `Extractor`                    | Resolves the client IP; safe for concurrent use                         |
| `HeaderGetter`                 | Multi-valued header access the extractor reads from                     |
| `HTTPHeader`                   | Adapts an `http.Header` to `HeaderGetter`                               |
| `NewContext` / `FromContext`   | Carry the resolved IP through a request context                         |
| `ParsePrefixes`                | Parses IP and CIDR strings into prefixes                                |

## Usage

```go
extractor, err := clientip.NewExtractor(clientip.WithTrustedPeers(netip.MustParsePrefix("35.191.0.0/16")))
if err != nil {
    return err
}
peer, _ := netip.ParseAddrPort(r.RemoteAddr)
ip := extractor.Extract(r.Context(), peer.Addr(), clientip.HTTPHeader(r.Header))
```
