# ping

```go
import ping "github.com/altessa-s/go-atlas/transport/http/server/handlers/ping"
```

Tiny static `/ping` HTTP handler. Always returns 200 with `{"message":"pong"}`. Useful when probe traffic must avoid touching real
dependencies (the more comprehensive option is `handlers/health`).

## Symbols

| Function / Type | Description |
|---|---|
| `Handler(rw)` | Writes `{"message":"pong"}` |
| `Response` | Response shape for the handler |
