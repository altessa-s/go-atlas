# core

Foundation packages for the Atlas framework. Packages under `core/` use only the Go standard library and other `core/*` packages — with narrow,
package-scoped exceptions: `io/spoolbudget` uses `golang.org/x/sync/semaphore`, and the Linux sandboxing packages (`runtime/capabilities`, `landlock`,
`nonewprivs`, `rlimits`, `seccomp`) use `golang.org/x/sys/unix` (see [docs/architecture.md](../docs/architecture.md)). Each package can be imported
independently without pulling in the rest of the module.

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
| [runtime](./runtime)           | Shutdown hooks, concurrency, signals, panics, sandboxing   |
| [scheduler](./scheduler)       | Task scheduling interface and priority model               |
| [text](./text)                 | String manipulation, interning, secure storage             |
| [time](./time)                 | Time formatting, calendar dates (`time/civil`), clocks (`time/clock`) |
| [types](./types)               | Bits, constraints, nil checks, optional, pointers, redacted strings, result |

## Design principles

- **Minimal dependencies** — the Go standard library, plus the package-scoped `golang.org/x` exceptions listed above.
- **Thread-safe by default** — all exported functions and types are safe for concurrent use unless documented otherwise.
- **Generic-first** — uses Go generics (`constraints.Integer`, `constraints.Primitive`, etc.) to minimize type-specific boilerplate.
- **Allocation-aware** — `sync.Pool` wrappers, zero-copy conversions, and iterator-based APIs to reduce GC pressure.
