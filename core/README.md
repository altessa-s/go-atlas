# core

Foundation packages for the Atlas framework. Every package under `core/` uses only the Go standard library — no third-party dependencies
— and each package can be imported independently without pulling in the rest of the module or any external code.

## Packages

| Package                        | Description                                                |
|--------------------------------|------------------------------------------------------------|
| [collections](./collections)   | Generic maps and slices utilities, iterators, pools        |
| [context](./context)           | Context timeout helpers                                    |
| [encoding](./encoding)         | Hashing, serialization, and entity-tags (ETags)            |
| [errors](./errors)             | Error wrapping, construction, and classification           |
| [factory](./factory)           | Base type for factory pattern implementations              |
| [io](./io)                     | Buffer pools, limited readers, file-system helpers, WAL    |
| [net](./net)                   | HTTP round-tripper adapter                                 |
| [retry](./retry)               | Stdlib-only retry loop with pluggable delay policies       |
| [runtime](./runtime)           | Finalizers, shutdown hooks, concurrency, signals, panics   |
| [scheduler](./scheduler)       | Task scheduling interface and priority model               |
| [text](./text)                 | String manipulation, interning, secure storage             |
| [time](./time)                 | Safe timer management and time formatting                  |
| [types](./types)               | Bits, constraints, nil checks, optional, pointers, redacted strings, result |

## Design principles

- **Zero external dependencies** — only the Go standard library.
- **Thread-safe by default** — all exported functions and types are safe for concurrent use unless documented otherwise.
- **Generic-first** — uses Go generics (`constraints.Integer`, `constraints.Primitive`, etc.) to minimize type-specific boilerplate.
- **Allocation-aware** — `sync.Pool` wrappers, zero-copy conversions, and iterator-based APIs to reduce GC pressure.
