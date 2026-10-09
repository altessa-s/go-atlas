# singleflight

```go
import "github.com/altessa-s/go-atlas/core/runtime/singleflight"
```

Deduplicates concurrent computations using Go key equality. Each caller can cancel its wait independently of the shared operation.

## Options

| Option              | Default | Meaning                                                   |
|---------------------|---------|-----------------------------------------------------------|
| `WithMaxConcurrent` | 1024    | Distinct active keys; non-positive disables the limit       |
| `WithTimeout`       | 15s     | Shared callback deadline; non-positive disables the timeout |

## Usage

```go
group := singleflight.New[string, User]()
user, err := group.Do(ctx, id, func(work context.Context) (User, error) {
    return loadUser(work, id)
})
```

New keys at capacity return `ErrOverloaded`; existing work can still be joined. There is no background admission queue. Callback contexts retain the
first caller's values and use the group deadline. Callbacks must honor cancellation; their capacity remains occupied until they actually exit.

Use [concurrency](../concurrency) for bounded processing of a slice rather than deduplicating calls by key.
