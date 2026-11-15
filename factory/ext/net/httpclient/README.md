# factory

```go
import "github.com/altessa-s/go-atlas/factory/ext/net/httpclient"
```

Maps configuration schemas to HTTP client and [observer](observe/README.md) options. Configuration stays independent of runtime clients.

## API

| Function             | Behavior                                                                                                            |
|----------------------|---------------------------------------------------------------------------------------------------------------------|
| `HealthOptions(cfg)` | Builds the health options of an `observe.New` observer; nil yields no options. Validate the schema before assembly. |
| `SSRFOptions(cfg)`   | Builds SSRF options; invalid CIDRs or egress rules return an error, nil preserves client protection defaults.       |

`SSRFOptions` maps `ClientSSRF` as follows: `disabled` yields `WithoutSSRFProtection` and ignores the rest; `allow` and `deny` rules are compiled
with `httpclient.ParseEgressPolicy` (errors wrap `httpclient.ErrInvalidEgressRule`) and installed with `WithEgressPolicy`; `allowedCIDRs` yield
`WithSSRFAllowedCIDRs`. With a policy, each allowed CIDR is an allow rule of it, after the `allow` rules and labeled `allowedCIDRs[i]`, so a
matching deny rule still refuses the address. Without rules the built-in private-address check applies, exempting the allowed CIDRs.

`HealthOptions` returns `[]observe.Option`; install them with the meter and health reporter:

```go
c := httpclient.New(httpclient.WithObserver(observe.New(meter, coord, factory.HealthOptions(cfg.Health)...)))
```

## Related packages

Proxy configuration is mapped by [`factory/ext/net/proxydial/README.md`](../proxydial/README.md). The corresponding
[client](https://github.com/altessa-s/go-atlas-net/tree/develop/http/client/README.md) owns runtime behavior and its full programmatic option surface.
