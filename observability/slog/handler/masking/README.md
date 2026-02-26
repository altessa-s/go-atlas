# masking

```go
import "github.com/altessa-s/go-atlas/observability/slog/handler/masking"
```

Package `masking` provides a `slog.Handler` middleware that masks sensitive data (credentials, PII) in log
records before passing them to an inner handler.

## Usage

```go
h := masking.NewHandler(inner,
    masking.WithDefaults(),                     // mask common fields (password, token, etc.)
    masking.WithField("ssn", masking.FullMask()),
    masking.WithField("email", masking.FixedMask("***")),
)
logger := slog.New(h)
```

## Functions

| Function / Option  | Description                                       |
|--------------------|---------------------------------------------------|
| `NewHandler`       | Wrap an inner handler with masking rules           |
| `WithDefaults`     | Register masks for common sensitive field names    |
| `WithField`        | Add a masking rule for a specific field            |
| `WithDefaultMask`  | Set the fallback mask for unmatched fields         |
| `FullMask`         | Replace entire value with mask characters          |
| `FixedMask`        | Replace value with a fixed string                  |
