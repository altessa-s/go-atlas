# factoryconv

```go
import "github.com/altessa-s/go-atlas/factory/transport/internal/factoryconv"
```

Shared config-to-runtime conversion helpers for the HTTP and gRPC server factories. Translates declarative `config` structures — regex pattern lists,
IP/CIDR prefixes, fallback behavior strings, IP and geographic ACL rules — into the runtime types consumed by transport middlewares and interceptors.

## Key helpers

| Helper                    | Purpose                                                                                  |
|---------------------------|------------------------------------------------------------------------------------------|
| `ServerTLS`               | Resolve the selected certificate provider and apply the minimum server TLS version       |
| `CompilePatterns`         | Compile string patterns to `[]*regexp.Regexp`; invalid patterns are silently skipped     |
| `StringMatcher`           | Match a string against an exact list (frozen in a `coremaps.ImmutableMap`) and compiled regex patterns; nil when both are empty |
| `IdempotencyKeyValidator` | Return the key validator of an `IdempotencyKeyFormat`: uuidv4 (or empty), opaque, or pattern; bounded by `maxLen` |
| `ConvertFallbackBehavior` | Map `middlewareconfig.FallbackBehavior` to `fallback.Behavior`; unknown values fail closed to deny |
| `BuildIpAclRegistry`      | Build an `ipacl.Registry` from default policy, rule list, and optional default rule      |
| `ConvertIpAclRule`        | Convert one `middlewareconfig.IPACLRule` to an `*ipacl.AccessRule`                           |
| `BuildGeoAclRegistry`     | Build a `geoacl.Registry` from default policy, rule list, and optional default rule      |
| `ConvertGeoAclRule`       | Convert one `middlewareconfig.GeoACLRule` to a `*geoacl.AccessRule`                          |

## Usage

```go
registry, err := factoryconv.BuildIpAclRegistry(c.DefaultPolicy, c.Rules, c.DefaultRule)
if err != nil {
    return fmt.Errorf("build IP ACL registry: %w", err)
}
```

## Error semantics

`BuildIpAclRegistry` and `BuildGeoAclRegistry` reject invalid endpoint regex patterns with an error; `ConvertIpAclRule` rejects invalid
allowlist/denylist prefixes. `CompilePatterns`, in contrast, drops invalid patterns silently — it backs best-effort ignore lists where a typo must not
prevent server startup.

See the sibling packages [`ipacl`](../../../../transport/policy/ipacl/), [`geoacl`](../../../../transport/policy/geoacl/),
[`fallback`](../../../../transport/policy/fallback/), and [`clientip`](https://github.com/altessa-s/go-atlas-net/tree/develop/clientip) for the target
runtime types.
