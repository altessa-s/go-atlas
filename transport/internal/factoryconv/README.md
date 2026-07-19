# factoryconv

```go
import "github.com/altessa-s/go-atlas/transport/internal/factoryconv"
```

Shared config-to-runtime conversion helpers for the HTTP and gRPC server factories. Translates declarative `config` structures — regex pattern
lists, IP/CIDR prefixes, fallback behavior strings, IP and geographic ACL rules — into the runtime types consumed by transport middlewares and
interceptors.

## Key helpers

| Helper                    | Purpose                                                                                  |
|---------------------------|------------------------------------------------------------------------------------------|
| `CompilePatterns`         | Compile string patterns to `[]*regexp.Regexp`; invalid patterns are silently skipped     |
| `ParsePrefixes`           | Parse IP/CIDR strings to `[]netip.Prefix` (alias of `clientip.ParsePrefixes`)            |
| `ConvertFallbackBehavior` | Map `config.FallbackBehavior` to `fallback.Behavior`; unknown values fail closed to deny |
| `BuildIpAclRegistry`      | Build an `ipacl.Registry` from default policy, rule list, and optional default rule      |
| `ConvertIpAclRule`        | Convert one `config.IpAclRuleConfig` to an `*ipacl.AccessRule`                           |
| `BuildGeoAclRegistry`     | Build a `geoacl.Registry` from default policy, rule list, and optional default rule      |
| `ConvertGeoAclRule`       | Convert one `config.GeoAclRuleConfig` to a `*geoacl.AccessRule`                          |

## Usage

```go
registry, err := factoryconv.BuildIpAclRegistry(c.DefaultPolicy, c.Rules, c.DefaultRule)
if err != nil {
    return fmt.Errorf("build IP ACL registry: %w", err)
}
```

## Error semantics

`BuildIpAclRegistry` and `BuildGeoAclRegistry` reject invalid endpoint regex patterns with an error; `ConvertIpAclRule` rejects invalid
allowlist/denylist prefixes. `CompilePatterns`, in contrast, drops invalid patterns silently — it backs best-effort ignore lists where a typo
must not prevent server startup.

See the sibling packages [`ipacl`](../ipacl/), [`geoacl`](../geoacl/), [`fallback`](../fallback/), and [`clientip`](../clientip/) for the
target runtime types.
