# handlers

Operational HTTP handlers shipped with the server. Each subpackage covers one concern; mount the ones you need on your router. The
layout mirrors `transport/grpc/handlers`.

## Subpackages

| Package | Mounts | Purpose |
|---|---|---|
| [`health`](./health) | `/healthz`, `/readyz`, `/healthz/details` | Liveness/readiness probes (static stubs and coordinator-backed) |
| [`ping`](./ping) | `/ping` | Tiny `{"message":"pong"}` connectivity check |
| [`pprof`](./pprof) | `/pprof/*` | Go runtime/pprof debug endpoints (privileged, opt-in) |
| [`metrics`](./metrics) | `/metrics` | Prometheus default-gatherer endpoint |

The HTTP server factory (`transport/http/server/factory`) wires all four under the configured internal prefix when the corresponding
toggle is on. See [docs/health.md](../../../../docs/health.md) for the broader picture.
