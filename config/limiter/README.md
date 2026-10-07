# limiterconfig

```go
import limiterconfig "github.com/altessa-s/go-atlas/config/limiter"
```

Package `limiterconfig` defines rate limiter schemas: token bucket, request budget and request limiter. Schemas are populated by
[`config/loader`](../loader) and consumed by the component factories, which map them to generated options; runtime packages never import them.

## Key types

| Type                            | Description                                                    |
|---------------------------------|----------------------------------------------------------------|
| `Budget`                 | Configures a distributed budget limiter for outbound requests. |
| `TokenBucketDefaultRule` | Defines the default rate limiting settings.                    |
| `TokenBucketTargetRule`  | Defines rate limiting settings for specific targets.           |
| `TokenBucketRules`       | Defines the complete set of rate limiting rules.               |
| `TokenBucket`            | Defines the configuration for token-bucket rate limiting.      |
| `RequestRate`               | Represents the configuration for rate limiting HTTP requests.  |

See the [config index](../README.md) for the other schema packages.
