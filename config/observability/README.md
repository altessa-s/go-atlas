# observabilityconfig

```go
import observabilityconfig "github.com/altessa-s/go-atlas/config/observability"
```

Package `observabilityconfig` defines logging, metrics, tracing and health schemas and the aggregate Observability block. Schemas are populated by
[`config/loader`](../loader) and consumed by the component factories, which map them to generated options; runtime packages never import them.

## Key types

| Type                  | Description                                                                                      |
|-----------------------|--------------------------------------------------------------------------------------------------|
| `Health`              | Defines the configuration for health check coordination.                                         |
| `LoggerConsoleOutput` | Represents the console output destination for logging.                                           |
| `LoggerLevel`         | Represents the minimum logging level for message output.                                         |
| `LogFormat`           | Represents the output format for log messages.                                                   |
| `LoggerBuffer`        | Configures asynchronous buffered logging.                                                        |
| `LoggerMaskRule`      | Defines a masking rule for specific fields or patterns.                                          |
| `Logger`              | Configures logging behavior for applications.                                                    |
| `MetricsType`         | Represents the type of metrics backend.                                                          |
| `Metrics`             | Represents the configuration for the metrics system.                                             |
| `MetricsAdapters`     | Contains configurations for different metrics adapters.                                          |
| `MetricsPrometheus`   | Contains Prometheus-specific configuration.                                                      |
| `Config`       | Represents the configuration for observability features including metrics, tracing, and logging. |
| `TracingType`         | Represents the type of tracing backend.                                                          |
| `SamplerType`         | Represents the type of trace sampler.                                                            |
| `OTLPProtocol`        | Represents the OTLP transport protocol.                                                          |
| `Tracing`             | Represents the configuration for the tracing system.                                             |
| `TracingAdapters`     | Contains configurations for different tracing adapters.                                          |
| `TracingSampler`      | Contains sampling configuration.                                                                 |
| `TracingOTLP`         | Contains OTLP-specific configuration.                                                            |
| `TracingConsole`      | Contains console output configuration.                                                           |

See the [config index](../README.md) for the other schema packages.
