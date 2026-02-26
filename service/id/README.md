# id

```go
import "github.com/altessa-s/go-atlas/service/id"
```

Package `id` provides stable, unique Service ID generation and management for distributed applications. A Service ID
uniquely identifies a running instance across restarts and deployments. The package ships three built-in providers
(`Env`, `File`, `Static`) and a cascading constructor that tries an environment variable first, then falls back to
file-based persistence.

## Usage

```go
// Cascading: env var first, then file-based persistence
s, err := id.New("/tmp/service.id", "SERVICE_ID")
if err != nil {
    log.Fatal(err)
}
fmt.Println("Service ID:", s.ID())

// Must variant for top-level init
s := id.MustNew("/tmp/service.id", "SERVICE_ID")
```

## Providers

| Provider | Constructor             | Description                                                                              |
|----------|-------------------------|------------------------------------------------------------------------------------------|
| `Env`    | `NewWithEnvProvider`    | Reads the ID from a named environment variable; returns `ErrInvalidEnv` when unset       |
| `File`   | `NewWithFileProvider`   | Reads or auto-generates a random ID persisted to a file on disk, survives restarts       |
| `Static` | `NewStaticProvider`     | Wraps a caller-supplied constant string, useful for tests and compile-time configuration |

## Key types

| Type / Interface | Description                                                                                         |
|------------------|-----------------------------------------------------------------------------------------------------|
| `Service`        | Primary entry point — delegates to an underlying Provider, created via `New` or `MustNew` helpers   |
| `Provider`       | Interface with a single method `ID() string`; must be safe for concurrent use                       |
| `Env`            | Provider that reads the ID from a process environment variable at construction time                  |
| `File`           | Provider that reads or generates a random ID persisted to a file on disk for durability             |
| `Static`         | Provider that always returns a caller-supplied constant string                                       |