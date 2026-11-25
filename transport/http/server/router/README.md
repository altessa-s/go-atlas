# router

```go
import "github.com/altessa-s/go-atlas/transport/http/server/router"
```

Package `router` defines the `Router` and `Route` interfaces that abstract HTTP routing functionality. These interfaces decouple the server from any
specific routing library, so implementations can be swapped without changing handler code. A `Router` supports handler and middleware registration,
path-prefix matching, and subrouter creation.

## Key types

| Type / Interface | Description                                                                            |
|------------------|----------------------------------------------------------------------------------------|
| `Router`         | Interface: `Handle`, `HandleFunc`, `Methods`, `PathPrefix`, `Use`, `Subrouter`         |
| `Route`          | Interface: `Handler`, `HandlerFunc`, `Methods`, `Path`, `PathPrefix`, `Subrouter`      |
| `Middleware`      | Function type: `func(http.Handler) http.Handler`                                       |

## Subpackages

| Package                | Description                                                                    |
|------------------------|--------------------------------------------------------------------------------|
| [std](./std)           | Router implementation using Go 1.22+ `http.ServeMux` enhanced patterns         |

`RouteTemplate` extracts the registered path template from `http.Request.Pattern` and returns `/_unknown` when no template is available.
Custom routers provide route-aware middleware with a template by populating the field before invoking the middleware chain.
