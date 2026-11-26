# limiterconfig

```go
import limiterconfig "github.com/altessa-s/go-atlas/config/limiter"
```

Package `limiterconfig` defines rate limiter schemas: token bucket, request budget and request limiter. Schemas are populated by
[`go-nomos/loader`](https://github.com/altessa-s/go-nomos/tree/develop/loader) and consumed by the component factories, which map them to generated
options; runtime packages never import them.

Both limiter schemas default to refusing new keys when memory storage is full of live windows. Set `insecureEvictLRU: true` to allow eviction
of live windows. Key churn can then reset another key's quota before that window expires.

## Key types

| Type                            | Description                                                    |
|---------------------------------|----------------------------------------------------------------|
| `Budget`                 | Configures a distributed budget limiter for outbound requests. |
| `TokenBucketDefaultRule` | Defines the default rate limiting settings.                    |
| `TokenBucketTargetRule`  | Defines rate limiting settings for specific targets.           |
| `TokenBucketRules`       | Defines the complete set of rate limiting rules.               |
| `TokenBucket`            | Defines the configuration for token-bucket rate limiting.      |
| `NATSContention`         | Tunes NATS storage CAS retries (`natsContention` in both limiters). |
| `RequestRate`               | Represents the configuration for rate limiting HTTP requests.  |

See the [config index](../README.md) for the other schema packages.
