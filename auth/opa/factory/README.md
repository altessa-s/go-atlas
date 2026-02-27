# factory

```go
import "github.com/altessa-s/go-atlas/auth/opa/factory"
```

Package `factory` builds `opa.Manager` instances and their `opa.PolicySource` backends from `config.OPA` configuration objects. Infrastructure
references (scheduler, health coordinator) are injected once at construction and reused across every manager the factory creates.

## Methods

| Method                     | Description                                                         |
|----------------------------|---------------------------------------------------------------------|
| `CreateManagerFromConfig`  | Create a fully wired OPA manager with source, watcher, and options  |
| `CreateSourceFromConfig`   | Create a PolicySource for manual manager construction               |

## Options

| Option                   | Default   | Description                                          |
|--------------------------|-----------|------------------------------------------------------|
| `WithLogger`             | discard   | Structured logger (`*slog.Logger`)                   |
| `WithScheduler`          | nil       | Task registrar for periodic policy update cycles     |
| `WithHealthCoordinator`  | nil       | Register created managers with health coordinator    |
