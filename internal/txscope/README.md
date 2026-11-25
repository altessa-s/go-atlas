# txscope

```go
import "github.com/altessa-s/go-atlas/internal/txscope"
```

The [PostgreSQL adapter](../../store/postgres/README.md) records savepoint lifetimes in the context. The [unit of work](../../messaging/eventbus/uow/README.md) checks those lifetimes before applying registered hooks and effects.

## Lifecycle

| Function         | Purpose                                                 |
|------------------|---------------------------------------------------------|
| `Begin`          | Open a nested scope and obtain its completion callback  |
| `Current`        | Identify the innermost savepoint of a context           |
| `Seal`           | End use of a scope before its SQL outcome is known     |
| `OpenWithin`     | Check whether a registration can join an active scope   |
| `ReleasedWithin` | Check the outcome of every enclosing savepoint          |
| `Key`, `Detach`  | Mark and hide transaction values from callbacks        |
| `WithCommitReporting`, `StartCommitReport`, `ReportFrom` | Bind a commit confirmation to the adapter that supplies the body context |
