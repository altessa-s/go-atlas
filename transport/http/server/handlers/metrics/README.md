# metrics

```go
import metricsh "github.com/altessa-s/go-atlas/transport/http/server/handlers/metrics"
```

Mounts a metrics exposition handler at `/metrics/` on a `router.Router` (serving bare `/metrics` needs a redirect such as Gorilla's
`StrictSlash(true)`, which the server factory enables). The handler comes from the metrics backend — `promadapter.DefaultHandler()` serves the
Prometheus default gatherer without compression — so the package depends on no metrics library.

## Symbols

| Function | Description |
|---|---|
| `Mount(r, handler) router.Router` | Mounts `handler` at `/metrics/` on `r` and returns the subrouter |
