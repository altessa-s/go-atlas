# handlers

Operational HTTP handlers shipped with the server. Each subpackage covers one concern; mount the ones you need on your router. The layout mirrors
`transport/grpc/handlers`.

## Subpackages

| Package | Mounts | Purpose |
|---|---|---|
| [`health`](./health) | `/healthz`, `/readyz`, `/healthz/details` | Liveness/readiness probes (static stubs and coordinator-backed) |
| [`ping`](./ping) | `/ping` | Tiny static `pong` connectivity check |
| [`pprof`](./pprof) | `/pprof/*` | Go runtime/pprof debug endpoints (privileged, opt-in) |
| [`metrics`](./metrics) | `/metrics` | Prometheus default-gatherer endpoint |

The HTTP server factory (`transport/http/server/factory`) mounts `/ping` and the static `/healthz` and `/readyz` stubs under the configured internal
prefix unless built-in handlers are disabled; `/healthz/details` is not mounted. It mounts `metrics` and `pprof` when their toggles are on and only on
the Gorilla router it creates itself — not when a router is supplied with `WithRouter`. See
[docs/observability/health.md](../../../../docs/observability/health.md) for the broader picture.
