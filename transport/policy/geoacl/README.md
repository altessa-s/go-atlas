# geoacl

```go
import "github.com/altessa-s/go-atlas/transport/policy/geoacl"
```

Package geoacl provides geographic access control logic shared by the gRPC interceptor and HTTP middleware layers.  A [Registry] holds per-endpoint and pattern-based [AccessRule] entries.

The HTTP middlewares and gRPC interceptors use it; the server factories build it from configuration.

## Decision

`Registry.Evaluate` looks up the rule of the endpoint. Without a rule the registry policy (`PolicyDeny` or `PolicyAllow`) decides and the resolver
is not called. With a rule the location is checked in this order:

| Step | Condition                                                             | Verdict         |
|------|-----------------------------------------------------------------------|-----------------|
| 1    | region, country or continent in a deny list                           | deny            |
| 2    | region, country or continent in an allow list                         | allow           |
| 3    | rule has an allow list                                                | deny            |
| 4    | rule has deny lists only, or no lists                                 | registry policy |

A rule with a non-empty allow list is an allowlist for its endpoint under either registry policy: with `PolicyAllow` a location outside the list
is still denied. A resolver error is returned to the caller.

## Unknown locations and internal callers

A GeoIP database returns an empty `GeoInfo` and no error for loopback, private and link-local addresses, so kubelet probes and in-cluster callers
arrive with an unknown location (no country and no continent). An unknown location is on no list:

| Rule                  | Registry policy | Unknown location                   |
|-----------------------|-----------------|------------------------------------|
| deny lists only       | `PolicyAllow`   | allow: it is not on the deny list  |
| deny lists only       | `PolicyDeny`    | deny: the registry default         |
| has an allow list     | either          | deny: it is not on the allow list  |
| no rule               | `PolicyAllow`   | allow, the resolver is not called  |
| no rule               | `PolicyDeny`    | deny, the resolver is not called   |

A sanctions deny list under `PolicyAllow` therefore does not turn internal callers away. An allow list or a `PolicyDeny` registry does, and the
registry has no switch for it. Let internal callers through explicitly in the adapter instead: exempt the endpoints they call (`WithIgnorePaths`,
`WithIgnoreMethods`, `WithIgnorePatterns`), or exempt the callers by address with `WithAllowPrivateAddresses`, which is off by default.

A nil rule is refused at registration (`Register`, `RegisterPattern` and `RegisterEndpoints` panic).

## API

`ParsePolicy`, `GeoInfo`, `FullRegion`, `GeoResolver`, `Policy`, `AccessRule`, `Registry`, `NewRegistry`, `Evaluate` — see the package documentation.
