# gorilla

```go
import "github.com/altessa-s/go-atlas/transport/http/server/router/gorilla"
```

Package `gorilla` provides a `router.Router` implementation using `gorilla/mux`. Feature-rich routing with regex patterns, host matching,
and path variable extraction. Call `Underlying()` for direct access to the `*mux.Router` when advanced gorilla/mux features are needed.
