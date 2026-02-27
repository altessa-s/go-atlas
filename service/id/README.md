# id

```go
import "github.com/altessa-s/go-atlas/service/id"
```

Package `id` provides stable, unique Service ID generation and management for distributed applications.
A Service ID uniquely identifies a running instance across restarts and deployments. The package ships
three built-in providers (`Env`, `File`, `Static`) and a cascading constructor that tries an environment
variable first, then falls back to file-based persistence.

## Providers

| Provider | Constructor             | Description                                                                 |
|----------|-------------------------|-----------------------------------------------------------------------------|
| `Env`    | `NewWithEnvProvider`    | Reads the ID from a named environment variable; returns `ErrInvalidEnv`     |
| `File`   | `NewWithFileProvider`   | Reads or auto-generates a random ID persisted to a file on disk             |
| `Static` | `NewStaticProvider`     | Wraps a caller-supplied constant string, useful for tests and configuration |

## Key types

| Type / Interface | Description                                                                    |
|------------------|--------------------------------------------------------------------------------|
| `Service`        | Primary entry point -- delegates to an underlying Provider via `New`/`MustNew` |
| `Provider`       | Interface with a single method `ID() string`; must be safe for concurrent use  |
| `Env`            | Provider that reads the ID from a process environment variable                 |
| `File`           | Provider that reads or generates a random ID persisted to a file on disk       |
| `Static`         | Provider that always returns a caller-supplied constant string                 |
