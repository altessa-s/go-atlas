# std

```go
import "github.com/altessa-s/go-atlas/transport/http/server/router/std"
```

Package `std` provides a `router.Router` implementation using the standard library `http.ServeMux` with Go 1.22+ enhanced pattern matching. The router
is sealed on the first call to `ServeHTTP` or `Initialize` -- subsequent route registration panics. This design enables a single initialization of the
middleware chain and route table, avoiding synchronization on the hot path.

## Subrouters

A subrouter shares its parent's `http.ServeMux`: `r.PathPrefix("/internal").Subrouter()` registers its routes under the full path
(`/internal/...`), so several subrouters of one prefix coexist and any router of the tree serves the whole tree. Middleware added with `Use` on the
root wraps every request, unmatched ones included; on a subrouter it wraps that subrouter's routes and its own subrouters' routes. A pattern may carry
its method (`"GET /users/{id}"`) on a subrouter too; path wildcards are read with `r.PathValue`.

`http.ServeMux` patterns have no regular expressions, and a prefix route (`PathPrefix`, or a pattern ending in `/`) answers its bare path with a
redirect to the trailing-slash form (307).

Root middleware can read the registered pattern from `http.Request.Pattern` before dispatch. The field is empty for generated
redirects and unmatched requests at that point. Wildcard values are available after dispatch to the registered route.
