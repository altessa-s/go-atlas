# respwriter

```go
import "github.com/altessa-s/go-atlas/internal/transport/respwriter"
```

Response writer wrapper shared by the HTTP middlewares and the marker contract between the writer package and the error interceptor. It is
internal to the module.

## Key types

| Symbol             | Purpose                                                                                                          |
|--------------------|------------------------------------------------------------------------------------------------------------------|
| `Recorder`         | Records the final status, body size and a bounded body prefix; forwards `Flush`, `FlushError`, `Hijack`, `Push`  |
| `NewRecorder`      | Pooled constructor; return the recorder with `Release`                                                           |
| `StructuredMarker` | Implemented by wrappers that must pass an already structured response through                                    |
| `MarkStructured`   | Tells every `StructuredMarker` on the `Unwrap` chain that the response about to be written is structured          |
| `IsFinal`          | Reports whether a status is final (200 and above, or 101)                                                        |

## Behavior

- The first final status wins; informational statuses other than 101 are forwarded and not recorded.
- A flush that reaches the underlying writer commits the implicit 200, so the record matches the status the client received.
- A body buffer that grew past 64 KiB is dropped on `Release` instead of being pooled.
