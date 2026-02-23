# http

```go
import "github.com/altessa-s/go-atlas/core/net/http"
```

Package `http` provides foundational HTTP utilities that complement `net/http`.

## RoundTripperFunc

An adapter that lets ordinary functions implement `http.RoundTripper` — useful for middleware, transport decoration, and test stubs.

```go
rt := http.RoundTripperFunc(func(req *stdhttp.Request) (*stdhttp.Response, error) {
    req.Header.Set("User-Agent", "go-atlas")
    return stdhttp.DefaultTransport.RoundTrip(req)
})

client := &stdhttp.Client{Transport: rt}
```
