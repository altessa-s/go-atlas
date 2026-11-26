# tokenbucket

```go
import "github.com/altessa-s/go-atlas/coord/limiters/tokenbucket"
```

Package `tokenbucket` implements rule-based rate limiting using a sliding window algorithm. Supports IP/CIDR-based rules, client-specific limits, and
authentication token extraction with configurable storage backends and sophisticated rule matching that prioritizes more specific rules.

## Options

`New(defaults, storage, opts...)` takes the default `RateLimitSettings` applied when no rule matches and the counting `storages.Storage`.

| Option                       | Default | Description                        |
|------------------------------|---------|------------------------------------|
| `WithRules`                  | none    | IP/CIDR rules, most specific first |
| `WithIPCacheSize`            | 1000    | LRU cache capacity for IP lookup   |
| `WithIPv6KeyPrefixBits`      | 64      | IPv6 prefix sharing one bucket     |
| `WithExtractToken`           | noop    | Function to extract auth token     |
| `WithExtractClientIPAddress` | noop    | Function to extract client IP      |
| `WithClientService`          | nil     | Client-specific rate limit service |
| `WithMeter`                  | no-op   | Metrics collector                  |
| `WithLogger`                 | discard | Structured logger                  |

A client service can return `RateLimitSkip` to fall back to IP rules. `RateLimitUnlimited` bypasses storage for either a token or an IP rule.
Ordinary settings with a non-positive limit or period return `ErrInvalidArgument`. With NATS storage, the period must also fit within the bucket retention.

## Storage keys

An IPv4 client is counted per address. An IPv6 client is counted per `WithIPv6KeyPrefixBits` prefix (default `/64`), because a subscriber can rotate
through every address of its subnet; `128` counts each address separately. Rules still match the full address. A token-limited client is keyed by the
SHA-256 digest of its token, so a storage backend never holds a bearer token.

## Subpackages

| Package                                                | Description                  |
|--------------------------------------------------------|------------------------------|
| [factory](../../../factory/coord/limiters/tokenbucket) | Configuration-based creation |
| [storages/memory](../storages/memory)                  | In-memory storage backend    |
| [storages/redis](../storages/redis)                    | Distributed Redis storage    |
| [storages/nats](../storages/nats)                      | NATS KV storage backend      |
