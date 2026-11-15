# errstatustest

```go
import "github.com/altessa-s/go-atlas/transport/grpc/interceptors/errstatus/errstatustest"
```

Runs gRPC handler calls in unit tests through the [errstatus](../README.md) server interceptor, so a test asserts the status a client would
see: a domain error converted by the configured rules, a status the handler returned passed through, anything else `codes.Internal`.

## Key types

| Helper                                             | Purpose                                                                                         |
|----------------------------------------------------|-------------------------------------------------------------------------------------------------|
| `New(opts...) *Harness`                            | A harness with the server's errstatus options; safe for concurrent use, one per test package    |
| `(*Harness).Call(t, srv, method, fn) error`        | Runs `fn(t.Context())` as the unary RPC `method` of `srv` (its `StatusError` converter applies) |
| `Unary(t, h, srv, method, fn) (T, error)`          | `Call` for a call returning a response; the zero value on failure                               |
| `Invoke(t, h, srv, method, fn, req) (Resp, error)` | `Unary` for a handler method value and its request                                              |
| `(*Harness).Convert(t, err) error`                 | The client's view of `err` from a handler or helper called directly (no service converter)      |

## Usage

```go
var harness = errstatustest.New(
    errstatus.WithCanonicalAPIRules(),
    errstatus.WithFinalizer(errstatus.DefaultFinalizerWithDomain("svc")),
)

got, err := errstatustest.Invoke(t, harness, h, botv1.BotService_GetBot_FullMethodName, h.GetBot, req)
if status.Code(err) != codes.NotFound { … }
```
