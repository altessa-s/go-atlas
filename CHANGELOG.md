# Changelog

## Unreleased

### Fixed

- **Breaking — persisted idempotency keys.** Idempotency storage keys joined their components with `:` (`idk:{service}:{key}` in the HTTP
  and gRPC transports, `ibk:{consumer}:{key}` in the broker middleware, `{namespace}:{key}` in `data/idempotency.Keeper`), so distinct
  tuples shared a record — `(service "a:b", key "c")` and `("a", "b:c")` — and one request or tenant could replay another's stored result.
  Keys are now composed by the new `data/idempotency.ComposeKey` in a length-prefixed v2 format (`idk:v2:9:transport17:users.UserService36:…`),
  and the `Keeper` wraps every key as `("namespace", namespace, key)`, also without a namespace. The switch is a clean cut: v1 records are
  never read. Deploy the release against a new Redis key prefix, NATS KV bucket or equivalent storage namespace; while old and new replicas
  run side by side, and until the old records expire, the same operation can run on both versions, and a rollback carries the same risk.
  Where duplicates are unacceptable, quiesce intake, drain handlers and wait out the old retention window before resuming. See
  [data/idempotency](data/idempotency/README.md#rolling-out-the-v2-key-format).
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
