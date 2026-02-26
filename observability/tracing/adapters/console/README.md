# console

```go
import "github.com/altessa-s/go-atlas/observability/tracing/adapters/console"
```

Package `console` provides a console adapter for the Atlas tracing system. Outputs spans to stdout or stderr
in human-readable or JSON format. Designed for development and debugging.

## Usage

```go
adapter := console.New(
    console.WithWriter(os.Stderr),
    console.WithPrettyPrint(),
)

provider := tracing.New(tracing.WithAdapter(adapter))
```
