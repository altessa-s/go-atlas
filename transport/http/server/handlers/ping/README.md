# ping

```go
import ping "github.com/altessa-s/go-atlas/transport/http/server/handlers/ping"
```

Tiny static `/ping` HTTP handler. Always returns 200 with a `Response{Message: "pong"}` written through the `writer.ReadWriter`; with the default writer
and JSON codec the body is `{"data":{"message":"pong"}}`. Useful when probe traffic must avoid touching real dependencies (when you do want real checks,
use `handlers/health`).

## Symbols

| Function / Type | Description |
|---|---|
| `Handler(rw)` | Writes `Response{Message: "pong"}` through `rw` |
| `Response` | Response shape for the handler |
