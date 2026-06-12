# http

```go
import corehttp "github.com/altessa-s/go-atlas/core/net/http"
```

Package `http` provides foundational HTTP utilities that complement `net/http`. Currently exposes the `RoundTripperFunc` function adapter.

Import the package under the `corehttp` alias to avoid shadowing the standard library `net/http`.

## Key types

| Type               | Description                                                                                          |
|--------------------|------------------------------------------------------------------------------------------------------|
| `RoundTripperFunc` | Adapter that lets ordinary functions implement `http.RoundTripper` — middleware, transport decoration, test stubs |
