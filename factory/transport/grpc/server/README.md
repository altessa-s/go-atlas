# factory

```go
import "github.com/altessa-s/go-atlas/factory/transport/grpc/server"
```

Package `factory` provides a fluent builder for creating a gRPC `Server` and its interceptors from configuration. `ServerBuilder` uses deferred error
accumulation — errors from any step are collected and returned at `Build()` time.

## Quick Start

```go
srv, err := factory.New(cfg.Grpc).
    UseLogger(logger).
    UseTracer(tracer).
    UseLimiter(limiter).
    UseAuth(authFn, nil).
    WithInterceptors().
    Build()
```

### Excluding specific interceptors

```go
srv, err := factory.New(cfg.Grpc).
    UseLogger(logger).
    UseTracer(tracer).
    WithInterceptors(auth.ID, cache.ID).
    Build()
```

## Server interceptor order

With all components enabled, `WithInterceptors` resolves the following order from the incoming call to the handler.

```text
metadata → realip → requestid → tracing → logger → metrics → errstatus → recovery
         → requestid_validation (strict mode) → ipacl → geoacl → cache → auth
         → limiter → idempotency → fieldbehavior → fieldmask → protovalidator → health → handler
```

Request ID propagation makes the ID available to logs and finalizers. Strict mode rejects a missing or malformed ID inside recovery.
On the return path, recovery turns a panic into an error, `errstatus` converts and finalizes it, and each observer reads the outgoing status.
The same ordering covers early returns from access control, authentication, limits and validation.

## Methods

`WithAuthInterceptor` exempts only explicitly configured `ignoreMethods` or `ignorePatterns`. `WithRealIPInterceptor` honors forwarding headers from
configured `trustedPeers` or `trustedProxies`; `trustPrivatePeers: true` explicitly restores implicit private-peer trust.

`WithGeoAclInterceptor` maps `geoACL.allowPrivateAddresses` (default `false`) to `geoacl.WithAllowPrivateAddresses`. A GeoIP database has no
location for loopback and private addresses: a rule with deny lists only leaves such a client to `defaultPolicy`, a rule with an allow list denies
it. The key lets these clients through instead. It skips the geographic check, deny lists included, for private client addresses, so it is only
safe when the client IP source is trustworthy (`realIP` with `trustedPeers` or `trustedProxies`).

### Constructor

| Method | Description |
|--------|-------------|
| `New(cfg)` | Creates a `ServerBuilder` for the given gRPC config |

### Dependencies

| Method | Description |
|--------|-------------|
| `UseLogger` | Sets the logger for the builder and all created components |
| `UseDefaultLogger` | Sets the logger to `slog.Default()` |
| `UseTlsProviders` | Sets the TLS providers used for server TLS configuration |
| `UseCacheMetadataProcessor` | Sets the metadata processor forwarded to the cache interceptor |
| `UseCacheMethodPolicies` | Sets per-method cache policies |
| `UseTracer` | Sets the tracer used by the tracing interceptor |
| `UseMeter`     | Sets the `metrics.Meter` (a `metrics.Collector` also fits); without it the metrics interceptor falls back to `metrics.Noop()` |
| `Meter` | Returns the configured `metrics.Meter`, or `metrics.Noop()` when none was set |
| `UseLimiter` | Sets the rate limiter used by the limiter interceptor |
| `UseIdempotency` | Sets the idempotency keeper used by the idempotency interceptor |
| `UseCacher` | Sets the cacher used by the cache interceptor |
| `UseAuth(authFn, clientAuth)` | Sets the authentication function and optional client auth handler |
| `UseHealthChecker` | Sets the health checker used by the health interceptor |
| `UseGeoResolver` | Sets the geo resolver used by the geoacl interceptor |
| `UseReasonCode` / `UseReasonCodeResolver` | Supplies service-defined reason codes for buf validation field violations |
| `UseErrStatusOptions` | Adds code-supplied errstatus options (error rules, converters) to the configured interceptor, after the config-built ones |
| `UseLoggerOptions` | Adds code-supplied logger options (payload redactor, log codes) to the configured interceptor, after the config-built ones |
| `UseFieldBehaviorOptions` | Adds field-behavior options (`WithMethodKind` for unconventional method names, `WithSkipResponse`) to the stock interceptor |
| `UseIdempotencyOptions` | Adds code-supplied idempotency options (`WithEntityIDExtractor`, `WithStatusCreator`) after the config-built ones |
| `UseServerOptions` | Applies server options after the config-built ones. `WithGrpcOptions` appends to the factory's gRPC options |
| `UseFieldMaskOptions` | Adds field mask options (`WithMethodKind`, update/read extractors) to the configured interceptor, after the config-built ones |

### Configuration

| Method | Description |
|--------|-------------|
| `WithInterceptors(exclude ...interceptors.Interceptor)` | Creates all enabled interceptors from `cfg.Interceptors`; pass typed IDs (e.g., `auth.ID`) to skip specific ones |
| `WithLoggerInterceptor()` | Adds a structured request/response logging interceptor |
| `WithMetricsInterceptor()` | Adds a metrics collection interceptor (`metrics.Meter`-backed) |
| `WithTracingInterceptor()` | Adds a distributed tracing interceptor |
| `WithRealIPInterceptor()` | Adds a real client IP extraction interceptor |
| `WithRecoveryInterceptor()` | Adds a panic recovery interceptor |
| `WithRequestIDInterceptor()` | Adds a request ID propagation/generation interceptor |
| `WithLimiterInterceptor()` | Adds a rate limiting interceptor |
| `WithIdempotencyInterceptor()` | Adds an idempotency enforcement interceptor |
| `WithCacheInterceptor()` | Adds a response caching interceptor |
| `WithAuthInterceptor()` | Adds an authentication interceptor |
| `WithHealthInterceptor()` | Adds a health gate interceptor |
| `WithIpAclInterceptor()` | Adds an IP access control interceptor |
| `WithGeoAclInterceptor()` | Adds a geographic access control interceptor |
| `WithErrStatusInterceptor()` | Adds an error-to-status conversion interceptor |
| `WithFieldBehaviorInterceptor()` | Adds the AIP-203 `field_behavior` strip interceptor; no config toggle — `WithInterceptors` adds it unless `fieldbehavior.ID` is excluded |
| `WithFieldMaskInterceptor()` | Adds the update/read field mask interceptor from `interceptors.fieldMask` (disabled by default) |
| `WithBufValidatorInterceptor()` | Adds a buf protovalidate interceptor. `WithInterceptors` adds it unless `protovalidator.ID` is excluded. The optional `protovalidator` config controls diagnostics |
| `WithInterceptor(i ...)` | Adds pre-built interceptor instances |

### Terminal

| Method | Description |
|--------|-------------|
| `Build` | Assembles and returns the gRPC server with all registered interceptors |

`Build` resolves the interceptor dependency order the way the server does when it starts, so a chain that cannot start — an interceptor whose
required dependency is absent (the limiter needs `realIP`), a duplicate, a cycle — fails `Build` instead of `Start`.

### Code-supplied interceptor options

The configured interceptors stay in the stock chain; code adds what YAML cannot express through the `Use*Options` hooks instead of excluding an
interceptor and re-adding a copy:

```go
srv, err := factory.New(cfg.Grpc).
    UseLogger(logger).
    UseLoggerOptions(logger.WithPayloadRedactor(redact)).
    UseFieldBehaviorOptions(fieldbehavior.WithMethodKind("/x.v1.X/RecordRate", fieldbehavior.KindCreate)).
    UseErrStatusOptions(errstatus.WithErrorRules(rules...)).
    WithInterceptors().
    Build()
```

Fields marked with the protobuf `debug_redact` option need no code: `interceptors.logger.debugRedact` (default `true`) clears them from logged
payloads before a code-supplied payload redactor runs.
