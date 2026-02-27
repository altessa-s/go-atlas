# http

```go
import "github.com/altessa-s/go-atlas/core/net/http"
```

Package `http` provides foundational HTTP utilities that complement `net/http`. Currently exposes the `RoundTripperFunc` function adapter.

## RoundTripperFunc

An adapter that lets ordinary functions implement `http.RoundTripper` — useful for middleware, transport decoration, and test stubs.
