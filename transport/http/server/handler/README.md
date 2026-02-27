# handler

```go
import "github.com/altessa-s/go-atlas/transport/http/server/handler"
```

Package `handler` provides standard HTTP handlers for operational endpoints. Health-check handlers are `server.HandlerFunc` values that
write JSON via `writer.ReadWriter`. Debug and metrics helpers accept a `router.Router` and mount a subrouter at the conventional path.

## Handlers

| Handler             | Type     | Description                                                              |
|---------------------|----------|--------------------------------------------------------------------------|
| `Ping`              | health   | Simple liveness check, returns `{"status": "ok"}`                        |
| `K8sHealtz`         | health   | Kubernetes `/healthz` endpoint                                           |
| `K8sReadyz`         | health   | Kubernetes `/readyz` readiness endpoint                                  |
| `Pprof`             | debug    | Mounts Go pprof subrouter at `/pprof`                                    |
| `PrometheusMetrics` | metrics  | Mounts Prometheus metrics subrouter at `/metrics`                        |
