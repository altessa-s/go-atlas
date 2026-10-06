# Changelog

## Unreleased

### Fixed

- `observability/tracing/factory` now honors `otlp.protocol` (`http` was ignored, gRPC was always used) and `otlp.compression: false` (gzip was
  always on).

### Changed

- `observability/tracing/adapters/otlp.WithCompression` takes a `bool`; `WithCompression(false)` disables the default gzip compression.

### Removed

Helpers that duplicate the Go 1.26 standard library were deleted. Migrate call sites as follows (see [docs/go126-migration.md](docs/go126-migration.md)):

| Removed                                         | Replacement                                                        |
|-------------------------------------------------|--------------------------------------------------------------------|
| `core/errors.AsType`                            | `errors.AsType`                                                    |
| `core/types/ptr.Wrap`                           | `new(v)`                                                           |
| `core/collections/slices.Values`, `Backward`    | `slices.Values`, `slices.Backward`                                 |
| `core/collections/slices.Chunk`                 | `slices.Chunk` (panics when size < 1; chunk capacity is clipped)   |
| `core/collections/slices.Any`                   | `slices.ContainsFunc`                                              |
| `core/collections/maps.Keys`, `Values`          | `maps.Keys`, `maps.Values`                                         |
| `core/runtime.AddCleanup`, `Cleanup`            | `runtime.AddCleanup`, `runtime.Cleanup`                            |
| `core/runtime.ClearFinalizer`                   | `runtime.SetFinalizer(obj, nil)`                                   |
| `core/time.TimerStopAndDrain` (package removed) | `t.Stop()` — timer channels are synchronous with `go ≥ 1.23`       |

Timer debounce paths (`plugins`, `security/tlsutils/providers/file`, `core/retry`) now rely on synchronous timer channels, the default for every
module that can depend on go-atlas (`go ≥ 1.26`). Running with `GODEBUG=asynctimerchan=1` is not supported.
