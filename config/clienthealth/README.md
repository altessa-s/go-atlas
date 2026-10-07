# clienthealthconfig

```go
import clienthealthconfig "github.com/altessa-s/go-atlas/config/clienthealth"
```

Package `clienthealthconfig` defines the client health-check schema for outbound gRPC and HTTP clients. Schemas are populated by
[`config/loader`](../loader) and consumed by the component factories, which map them to generated options; runtime packages never import them.

## Key types

| Type                      | Description                                                                 |
|---------------------------|-----------------------------------------------------------------------------|
| `StateMapper` | Defines the mapping strategy from gRPC connection states to health status.  |
| `Config`            | Contains base health monitoring configuration for external service clients. |
| `HTTP`        | Contains health monitoring configuration for HTTP clients.                  |
| `GRPC`        | Contains health monitoring configuration for gRPC clients.                  |

See the [config index](../README.md) for the other schema packages.
