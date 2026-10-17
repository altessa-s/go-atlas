# metrics

```go
import metricsh "github.com/altessa-s/go-atlas/transport/http/server/handlers/metrics"
```

Mounts the Prometheus default-gatherer `/metrics/` endpoint on a `router.Router` (serving bare `/metrics` needs a redirect such as Gorilla's
`StrictSlash(true)`, which the server factory enables). Compression is intentionally disabled — Prometheus scrapers expect the raw exposition format.

## Symbols

| Function | Description |
|---|---|
| `Mount(r) router.Router` | Mounts `/metrics/` on `r` and returns the subrouter |
