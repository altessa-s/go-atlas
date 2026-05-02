# health

```go
import healthh "github.com/altessa-s/go-atlas/transport/http/server/handlers/health"
```

HTTP handlers for liveness and readiness probes. Static stubs work without any dependency; coordinator-backed handlers consult an
`observability/health.Coordinator` and return 503 when the service is not serving.

## Handlers

| Function | Path convention | Behavior |
|---|---|---|
| `K8sHealtz` | `/healthz` | Static `{"status":"ok"}`, HTTP 200 |
| `K8sReadyz` | `/readyz` | Static `{"status":"ok"}`, HTTP 200 |
| `Healthz(coord)` | `/healthz` | `coord.CheckStatus(ctx, ?service)`; 200 / 503 |
| `Readyz(coord)` | `/readyz` | `coord.CheckStatus(ctx, "")`; 200 only when overall is `SERVING` |
| `Detailed(coord)` | `/healthz/details` | `coord.ListStatuses`; per-service breakdown, 503 if any service down |

## Response shapes

| Type | Used by | Body |
|---|---|---|
| `Response` | `K8sHealtz`, `K8sReadyz`, `Healthz`, `Readyz` | `{"status":"SERVING"}` |
| `ServiceResponse` | `Detailed` | `{"status":"...", "services":{...}}` |
| `ServiceStatus` | nested in `ServiceResponse` | `{"status":"..."}` |

See [docs/observability/health.md](../../../../../docs/observability/health.md) for the broader picture (Coordinator API, gRPC twin, lifecycle).
