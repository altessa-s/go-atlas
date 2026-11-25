# grpcclientconfig

```go
import grpcclientconfig "github.com/altessa-s/go-atlas/config/grpcclient"
```

Package `grpcclientconfig` defines the name resolution and connection backoff schema of outbound gRPC clients. Schemas are populated by
[`go-nomos/loader`](https://github.com/altessa-s/go-nomos/tree/develop/loader) and mapped to [client](../../transport/grpc/client) options by
`ConnectionOptions` in [`factory/transport/grpc/client`](../../factory/transport/grpc/client); runtime packages never import them.

```yaml
grpcClient:
  resolver:
    refreshInterval: 15s
    backoff:
      maxDelay: 3s
  connectBackoff:
    maxDelay: 10s
```

## Key types

| Type              | Description                                                                                     |
|-------------------|-------------------------------------------------------------------------------------------------|
| `Config`          | The `resolver` and `connectBackoff` sections of one client                                       |
| `Resolver`        | The [DNS resolver](../../transport/grpc/client/dnsresolver) the client installs, or `builtin` to keep grpc-go's |
| `ResolverBackoff` | Backoff between failed or empty lookups                                                          |
| `ConnectBackoff`  | Backoff between attempts to connect to one address                                               |

## Fields

| Field                                  | Default            | Meaning                                                                                  |
|----------------------------------------|--------------------|------------------------------------------------------------------------------------------|
| `resolver.builtin`                     | false              | Use grpc-go's built-in dns resolver; the other resolver fields are ignored               |
| `resolver.refreshInterval`             | 30s                | Re-resolution interval while lookups succeed (minimum 1s)                                 |
| `resolver.refreshJitter`               | unset (0.1)        | Largest random shortening of a refresh interval (0-0.5); 0 disables jitter               |
| `resolver.minInterval`                 | 2s                 | Shortest pause before a re-resolution gRPC requests; at most `refreshInterval`            |
| `resolver.lookupTimeout`               | 10s                | Timeout of one lookup                                                                     |
| `resolver.backoff.baseDelay`           | 1s                 | Delay after the first failed lookup                                                       |
| `resolver.backoff.multiplier`          | 1.6                | Growth per further failure (minimum 1)                                                    |
| `resolver.backoff.jitter`              | unset (0.2)        | Randomized fraction of each delay (0-1); 0 disables jitter                                |
| `resolver.backoff.maxDelay`            | 5s                 | Cap of the delay; not below `baseDelay`                                                   |
| `connectBackoff.baseDelay`             | 1s                 | Delay after the first failed connection attempt                                           |
| `connectBackoff.multiplier`            | 1.6                | Growth per further failure (minimum 1)                                                    |
| `connectBackoff.jitter`                | unset (0.2)        | Randomized fraction of each delay (0-1); 0 disables jitter                                |
| `connectBackoff.maxDelay`              | 2m                 | Cap of the delay; not below `baseDelay`                                                   |

A zero duration or multiplier and an unset jitter keep the client default, so `Config{}` and an omitted section change nothing. Negative durations,
a multiplier below 1, a backoff jitter outside 0-1, a refresh jitter outside 0-0.5, a base delay above its cap and a minimum interval above the
refresh interval fail `Validate`.

See the [config index](../README.md) for the other schema packages.
