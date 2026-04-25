# std

```go
import "github.com/altessa-s/go-atlas/transport/http/server/router/std"
```

Package `std` provides a `router.Router` implementation using the standard library `http.ServeMux` with Go 1.22+ enhanced pattern
matching. The router is sealed on the first call to `ServeHTTP` or `Initialize` -- subsequent route registration panics. This design
enables a single initialization of the middleware chain and route table, avoiding synchronization on the hot path.
