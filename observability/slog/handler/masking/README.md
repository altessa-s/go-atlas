# masking

```go
import "github.com/altessa-s/go-atlas/observability/slog/handler/masking"
```

Package `masking` provides a `slog.Handler` middleware that masks sensitive data (credentials, PII) in log records before passing them to
an inner handler. Supports full masks, fixed replacement strings, and per-field masking rules.

## Functions

| Function / Option  | Description                                       |
|--------------------|---------------------------------------------------|
| `NewHandler`       | Wrap an inner handler with masking rules; skips invalid patterns |
| `New`              | Like `NewHandler`, but returns an error wrapping `ErrInvalidPattern` for each empty or non-compiling pattern |
| `WithDefaults`     | Register masks for common sensitive field names    |
| `WithField`        | Add a masking rule for a specific field            |
| `WithDefaultMask`  | Mask for rules registered with a nil `MaskFunc`    |
| `FullMask`         | Replace entire value with mask characters          |
| `FixedMask`        | Replace value with a fixed string                  |
| `URLMask`          | Mask sensitive parts of URLs (buckets, paths)      |
| `S3URLMask`        | Redact S3/cloud storage URLs for safe logging      |
