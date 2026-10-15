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
| `WithTrustedPeers`        | none                      | Peer prefixes whose forwarding headers are honored                 |
| `WithTrustedProxies`      | none                      | Trusted peer prefixes, also skipped as forwarded proxy hops         |
| `WithTrustPrivatePeers`   | `false`                   | Restore implicit trust of private and reserved peers                |
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

Forwarding headers are ignored unless the connection peer belongs to `WithTrustedPeers` or `WithTrustedProxies`. Private, loopback and reserved
addresses are not trusted automatically. Configure the CIDRs of your reverse proxies explicitly. `WithTrustPrivatePeers()` restores the previous
implicit trust behavior; use it only when every peer in those ranges is allowed to supply forwarding headers. HTTP and gRPC realip use this same
policy. When limiting `X-Forwarded-For` to `MaxIPsPerHeader` entries, the extractor retains the rightmost suffix, including proxy-appended addresses.
With a nonzero `WithTrustedProxiesCount`, empty or malformed hops retain their positions. If the selected client is invalid, private or outside
the retained suffix, extraction returns the connection peer instead of scanning another position or header. IPv4-mapped header addresses are
returned as native IPv4; mapped IPv4 trust prefixes are normalized consistently. Peer fallback preserves the supplied peer representation.

```go
extractor, err := clientip.NewExtractor(clientip.WithTrustedPeers(netip.MustParsePrefix("35.191.0.0/16")))
if err != nil {
    return err
}
peer, _ := netip.ParseAddrPort(r.RemoteAddr)
ip := extractor.Extract(r.Context(), peer.Addr(), clientip.HTTPHeader(r.Header))
```
