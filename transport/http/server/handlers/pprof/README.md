# pprof

```go
import pprofh "github.com/altessa-s/go-atlas/transport/http/server/handlers/pprof"
```

Mounts Go's `net/http/pprof` debug endpoints on a `router.Router`. Opt-in only — see [Security](#security) before mounting.

## Security

`Mount` performs NO authentication or authorization. The endpoints MUST be placed behind an auth middleware or bound to a non-public
listener (localhost or an internal admin port). Exposed publicly, they hand any caller heap and goroutine dumps and CPU profiles —
which can contain secrets held in process memory — and enable cheap denial of service by triggering expensive profile collection.

## Symbols

| Function | Description |
|---|---|
| `Mount(r) router.Router` | Mounts `/pprof/*` on `r` and returns the subrouter |

Endpoints exposed under `/pprof`: `/`, `/cmdline`, `/profile`, `/symbol`, `/trace`, `/allocs`, `/block`, `/goroutine`, `/heap`,
`/mutex`, `/threadcreate`.
