# s3config

```go
import s3config "github.com/altessa-s/go-atlas/config/s3"
```

Package `s3config` defines the S3 connection schema shared by TLS certificate providers and OPA bundle sources. Schemas are populated by
[`config/loader`](../loader) and consumed by the component factories, which map them to generated options; runtime packages never import them.

## Key types

| Type | Description                                                                   |
|------|-------------------------------------------------------------------------------|
| `Config` | Represents the configuration for Amazon S3 or S3-compatible storage services. |

See the [config index](../README.md) for the other schema packages.
