# handler

Custom `slog.Handler` implementations for the Atlas framework. Each handler adds a specific processing stage — buffering, colorized rendering,
per-subsystem level filtering, sensitive data masking, prefixing, or fan-out — composable into a middleware-style handler chain.

## Subpackages

| Package                    | Description                                                                        |
|----------------------------|------------------------------------------------------------------------------------|
| [buffered](./buffered)     | Writes records asynchronously; records at or above the bypass level are synchronous |
| [colorized](./colorized)   | Adds ANSI color output for development terminals                                   |
| [leveled](./leveled)       | Per-subsystem log level filtering based on the `subsystem` attribute               |
| [masking](./masking)       | Masks sensitive data (credentials, PII) in log records before passing to inner     |
| [multi](./multi)           | Fans out log records to multiple child handlers (delegates to stdlib on Go 1.26+)  |
| [prefixed](./prefixed)     | Merges string values under a configured key into one formatted prefix attribute    |
