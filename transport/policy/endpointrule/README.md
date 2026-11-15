# endpointrule

```go
import "github.com/altessa-s/go-atlas/transport/policy/endpointrule"
```

Package endpointrule provides the endpoint-to-rule resolution shared by the access-control registries in internal/transport (ipacl, geoacl).  A [Registry] maps endpoints to rules of any type through exact names, regular-expression patterns and an optional default.

The HTTP middlewares and gRPC interceptors use it; the server factories build it from configuration.

## Pattern matching

A pattern must match the **whole** endpoint. `RegisterPattern` recompiles it as `^(?:pattern)$`, so anchors of its own are optional and a pattern
never selects a rule by a substring:

| Pattern         | Matches                             | Does not match                        |
|-----------------|-------------------------------------|---------------------------------------|
| `/admin`        | `/admin`                            | `/x/admin`, `/admin/users`, `/admin/` |
| `/admin/?`      | `/admin`, `/admin/`                 | `/admin/users`                        |
| `/admin(/.*)?`  | `/admin`, `/admin/`, `/admin/users` | `/administrator`, `/x/admin`          |
| `/internal\..*` | `/internal.Debug/Dump`              | `/x/internal.Debug/Dump`              |

A trailing slash is part of the endpoint: `/admin` and `/admin/` are different endpoints, and the registry does not normalize either the pattern or
the endpoint. A pattern written as a prefix (`^/admin`) or a suffix (`/admin$`) now matches only the exact text; add `.*` where more may follow or
precede. Exact names registered with `Register` are compared as is and take precedence over patterns.

The HTTP middlewares look a rule up only for a canonical request path, the form a router dispatches on: a leading slash and no empty, `.` or `..`
segment, and no encoded slash in the escaped path. They refuse any other spelling (`//admin`, `/public/../admin`, `/v1%2Fadmin`) with 403 before
the lookup, so register rules for canonical, decoded paths.

## Nil rules

`Register`, `RegisterPattern` and `RegisterEndpoints` panic on a nil rule: it is a programming error that would otherwise surface as a nil
dereference on the first matching request. `SetDefault(nil)` removes the default rule.

## API

`Registry`, `Register`, `RegisterPattern`, `RegisterEndpoints`, `SetDefault`, `Lookup` — see the package documentation.
