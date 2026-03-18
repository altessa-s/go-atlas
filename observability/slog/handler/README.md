# handler

Custom `slog.Handler` implementations for the Atlas framework. Each handler wraps an inner handler and adds a specific processing
stage — buffering, colorization, sensitive data masking, or key prefixing — composable into a middleware-style handler chain.

## Subpackages

| Package                    | Description                                                                        |
|----------------------------|------------------------------------------------------------------------------------|
| [buffered](./buffered)     | Batches log records and flushes them periodically or on threshold                  |
| [colorized](./colorized)   | Adds ANSI color output for development terminals                                   |
| [leveled](./leveled)       | Per-subsystem log level filtering based on the `subsystem` attribute               |
| [masking](./masking)       | Masks sensitive data (credentials, PII) in log records before passing to inner     |
| [multi](./multi)           | Fans out log records to multiple child handlers (delegates to stdlib on Go 1.26+)  |
| [prefixed](./prefixed)     | Prepends a configurable key prefix to all log record attributes                    |
