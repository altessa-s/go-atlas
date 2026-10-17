# pluginsconfig

```go
import pluginsconfig "github.com/altessa-s/go-atlas/config/plugins"
```

Package `pluginsconfig` defines the plugin manager and plugin sandbox schemas. Schemas are populated by [`config/loader`](../loader) and consumed by the
component factories, which map them to generated options; runtime packages never import them.

## Key types

| Type                  | Description                                                                                                                        |
|-----------------------|------------------------------------------------------------------------------------------------------------------------------------|
| `Config`             | Defines configuration for the dynamic plugin manager.                                                                              |
| `Signature`    | Configures plugin signature verification.                                                                                          |
| `Sandbox`      | Configures Linux process-hardening primitives that the plugin manager applies before opening any .so file.                         |
| `Landlock`     | Is a strict filesystem allowlist applied via Linux Landlock (kernel >= 5.13).                                                      |
| `Capabilities` | Configures Linux capability dropping applied via the standalone [github.com/altessa-s/go-atlas/core/runtime/capabilities] package. |

See the [config index](../README.md) for the other schema packages.

## Snapshot storage

`snapshotDir` selects a trusted executable base directory for private plugin copies; empty uses the OS temp directory. Override it when the temp mount
is `noexec`. Protect its ownership, ancestors and ACLs from plugin producers. See [plugin security](../../plugins/README.md#security).
