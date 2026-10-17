# masking

```go
import "github.com/altessa-s/go-atlas/observability/slog/handler/masking"
```

Package `masking` provides a `slog.Handler` middleware that masks sensitive data (credentials, PII) in log records before passing them to an inner
handler. Supports full masks, fixed replacement strings, and per-field masking rules.

## Functions

| Function / Option  | Description                                       |
|--------------------|---------------------------------------------------|
| `NewHandler`       | Wrap an inner handler with masking rules; skips invalid patterns |
| `New`              | Like `NewHandler`, but returns an error wrapping `ErrInvalidPattern` for each empty or non-compiling pattern |
| `WithDefaults`     | Register masks for common sensitive field names and enable nested-field masking |
| `WithField`        | Add a masking rule for a specific field            |
| `WithPattern`      | Add a masking rule for field names matching a regex |
| `WithCaseSensitive` | Match field names case-sensitively (default: case-insensitive) |
| `WithMaskNestedFields` | Mask matching fields inside groups and nested values |
| `WithDefaultMask`  | Mask for rules registered with a nil `MaskFunc`    |
| `FullMask`         | Replace entire value with mask characters          |
| `FixedMask`        | Replace value with a fixed string                  |
| `PartialMask`, `SmartMask` | Keep the first/last characters and mask the rest |
| `EmailMask`, `PhoneMask`, `CreditCardMask` | Format-aware masks |
| `HashMask` | Replaces a non-empty value with an optional prefix plus `[HASH]` |
| `URLMask`          | Mask sensitive parts of URLs (buckets, paths)      |
| `S3URLMask`        | Redact S3/cloud storage URLs for safe logging      |

## Deferred values

`LogValuer` values are resolved before masking, including values bound with `WithAttrs` and nested in structs, maps, slices or groups. The resolved
representation is forwarded so downstream handlers do not invoke the original value again. Direct chains use slog's bounded resolution; nested
groups/containers and pointer chains share bounded traversal and emit a placeholder on exhaustion.
