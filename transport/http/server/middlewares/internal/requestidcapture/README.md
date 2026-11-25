# requestidcapture

```go
import "github.com/altessa-s/go-atlas/transport/http/server/middlewares/internal/requestidcapture"
```

HTTP recovery uses `WithCapture` to observe the ID that request ID middleware resolves inside its handler. Reading and recording the ID is safe across
goroutines, including handlers wrapped in `http.TimeoutHandler`.
