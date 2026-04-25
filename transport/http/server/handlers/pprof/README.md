# pprof

```go
import pprofh "github.com/altessa-s/go-atlas/transport/http/server/handlers/pprof"
```

Mounts Go's `net/http/pprof` debug endpoints on a `router.Router`. Opt-in only — pprof exposes heap, goroutine, CPU and trace
profiles. Treat it as privileged: bind it to an internal admin port or require auth.

## Symbols

| Function | Description |
|---|---|
| `Mount(r) router.Router` | Mounts `/pprof/*` on `r` and returns the subrouter |

Endpoints exposed under `/pprof`: `/`, `/cmdline`, `/profile`, `/symbol`, `/trace`, `/allocs`, `/block`, `/goroutine`, `/heap`,
`/mutex`, `/threadcreate`.
