# factory

```go
import "github.com/altessa-s/go-atlas/auth/scope/factory"
```

Builds a frozen [`scope.Registry`](../) from a [`config.ScopeRegistry`](../../../config). It is the config-driven counterpart to
constructing a registry by hand, giving the scope subsystem the same config→component path the [OPA factory](../../opa/factory) provides.

Only the action-key→required-scope table is declarative. The matcher and the authorizer stay in code, since they depend on the caller's
principal type, which has no place in configuration.

## API

| Symbol                          | Description                                                                            |
|---------------------------------|----------------------------------------------------------------------------------------|
| `New(cfg *config.ScopeRegistry)`| Create a `RegistryBuilder`. A nil cfg is accepted; the error surfaces at `Build`.       |
| `RegistryBuilder.Build()`       | Register every rule and return the frozen `*scope.Registry`.                            |

`Build` fails when the config is nil, when a rule lists no keys, or when an action key appears in more than one rule (an ambiguous policy
is a configuration error, not last-write-wins).

## Usage

```go
reg, err := factory.New(&cfg.Scope).Build() // cfg.Scope is a config.ScopeRegistry
if err != nil {
    return err
}

// The matcher and authorizer are code, because they depend on the principal type:
enf := scope.NewEnforcer(reg, scope.ScopeAuthorizer(scopesOf, scope.Exact()))
```

Example configuration:

```yaml
scope:
  rules:
    - scope: "files:read"
      keys: ["/files.v1.Files/Read", "/files.v1.Files/List"]
    - scope: "files:write"
      keys: ["/files.v1.Files/Write"]
    - scope: ""               # public — always allowed once authenticated
      keys: ["/health.v1.Health/Check"]
```

## See also

- [`auth/scope`](../) — the policy primitive this factory populates.
- [`docs/auth/scope.md`](../../../docs/auth/scope.md) — scope guide.
