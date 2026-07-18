# Logging (slog)

```go
import (
    slogx "github.com/altessa-s/go-atlas/observability/slog"

    slogfactory "github.com/altessa-s/go-atlas/observability/slog/factory"

    "github.com/altessa-s/go-atlas/observability/slog/handler/buffered"
    "github.com/altessa-s/go-atlas/observability/slog/handler/colorized"
    "github.com/altessa-s/go-atlas/observability/slog/handler/leveled"
    "github.com/altessa-s/go-atlas/observability/slog/handler/masking"
    "github.com/altessa-s/go-atlas/observability/slog/handler/multi"
    "github.com/altessa-s/go-atlas/observability/slog/handler/prefixed"
)
```

`observability/slog` adds what `log/slog` does not give you out of the box: nil-safe attribute helpers, a logger and field bag carried through
`context.Context`, a runtime `slog.LevelVar`, and six `slog.Handler` middlewares (async buffering, ANSI-colored terminal output,
per-subsystem level filtering, sensitive-field masking, fan-out, and key prefixing). `observability/slog/factory.LoggerBuilder` wires them
into a single chain from a `config.Logger` block, but every handler is importable on its own if you'd rather assemble your own.

Alias the root package as `slogx` so it doesn't collide with the standard library.

---

## What it does

| Feature                          | How it works                                                                                          |
|----------------------------------|-------------------------------------------------------------------------------------------------------|
| Nil-safe attribute helpers       | `String`, `Int`, `Int64`, `Error` — return an empty (ignored) attr on nil pointers or empty strings    |
| Context-scoped logger            | `ContextWithLogger` / `FromContext` / `FromContextOrDefault` carry a `*slog.Logger` through requests   |
| Context-scoped fields            | `InjectFields` + `AppendField`/`AppendFields` accumulate request-scoped fields between layers          |
| Runtime level switch             | `slogx.GlobalLevel` (`*slog.LevelVar`) — flip levels for every factory-built logger at once            |
| Subsystem identity               | `slogx.Module("auth")` stamps the `subsystem` attribute used by the `leveled` handler                  |
| Graceful shutdown                | `slogx.Shutdown(ctx, logger)` walks the chain and drains any buffered handler                          |
| Async buffering with bypass      | `buffered` handler — background worker, sync write at or above `BypassLevel` (default `Error`)         |
| Colorized terminal               | `colorized` handler — ANSI colors, prefix-as-tag, attribute-color map, optional source locations       |
| Per-subsystem filtering          | `leveled` handler — different levels per `subsystem` attribute, with a single-comparison fast path     |
| PII / credential masking         | `masking` handler — exact field names, regex patterns, nested-group descent, smart masks per type      |
| Fan-out                          | `multi` handler — same record to several backends (file + stderr + remote); delegates to stdlib on 1.26+ |
| Key prefixing                    | `prefixed` handler — pulls a configured key out of the record and formats it as `[tag]` or JSON-friendly |
| YAML factory                     | `factory.LoggerBuilder` — assembles the full chain from `config.Logger` with deferred error accumulation |

---

## How it's wired

The factory composes handlers in a fixed order, inside-out from the writer back to the logger. Each layer is added only when its
configuration is non-empty, so a minimal config produces a minimal chain.

```
slog.Logger
   │
   ▼
[buffered]           (cfg.Buffer.Enabled — async worker, bypass on Error)
   │
   ▼
[masking]            (cfg.MaskRules / SensitiveTags / EnableDefaultMasks / WithEnableMasking)
   │
   ▼
[leveled]            (cfg.Subsystems non-empty — per-subsystem level filter)
   │
   ▼
[prefixed]           (always — pulls cfg.AppGroupName / module key into a tag)
   │
   ▼
[colorized] | JSON   (cfg.OutputFormat: "text" → colorized, "json" → slog.JSONHandler)
   │
   ▼
os.Stdout / os.Stderr   (cfg.Output)
```

Records flow top-down. The `app` group (`name` / `version` / `sid`) and any `cfg.Tags` are attached to the `slog.Logger` itself after `Build`
returns, via `Logger.With`, so they ride on every record in the chain.

---

## Quick start

### Minimal — JSON to stdout from config

```go
package main

import (
    "github.com/altessa-s/go-atlas/config"

    slogfactory "github.com/altessa-s/go-atlas/observability/slog/factory"
)

func main() {
    cfg := config.DefaultLogger()
    cfg.Level = config.LoggerLevelInfo
    cfg.OutputFormat = config.LogFormatJSON

    logger, err := slogfactory.New(&cfg).Build()
    if err != nil {
        panic(err)
    }
    logger.Info("service started")
}
```

### Context-scoped logger and fields

```go
import (
    "context"

    slogx "github.com/altessa-s/go-atlas/observability/slog"
)

// In an HTTP/gRPC interceptor at the edge of the request.
func WithLogging(ctx context.Context, base *slog.Logger, requestID string) context.Context {
    ctx = slogx.InjectFields(ctx, slogx.Fields{
        {Key: "request_id", Value: requestID},
    })
    return slogx.InjectLogger(ctx, base) // base + injected fields
}

// Deeper layers (no plumbing needed):
func handle(ctx context.Context) {
    slogx.AppendField(ctx, "user_id", uid) // visible to every downstream FromContext
    slogx.FromContextOrDefault(ctx).Info("handled")
}
```

`ContextWithLogger` is no-op when a logger is already in `ctx`. `FromContext` returns `nil` if absent; use `FromContextOrDefault` in code
that always needs a logger.

### Standalone handlers (no factory)

```go
import (
    "log/slog"
    "os"

    "github.com/altessa-s/go-atlas/observability/slog/handler/colorized"
    "github.com/altessa-s/go-atlas/observability/slog/handler/masking"
)

h := slog.Handler(colorized.NewHandler(os.Stderr,
    colorized.WithLevel(slog.LevelDebug),
    colorized.WithAddSource(),
))
h = masking.NewHandler(h, masking.WithDefaults()) // PII redaction on top
slog.SetDefault(slog.New(h))
```

### Graceful shutdown

```go
import slogx "github.com/altessa-s/go-atlas/observability/slog"

if err := slogx.Shutdown(ctx, logger); err != nil {
    // log buffer drain failed
}
```

When the factory enables `cfg.Buffer.Enabled`, `Build` registers `slogx.Shutdown(ctx, logger)` with `core/runtime.OnShutdown` automatically;
call it manually if you compose the chain yourself.

---

## Root-package API (`observability/slog`)

### Nil-safe attribute helpers

| Helper                                | Behavior                                                                            |
|---------------------------------------|-------------------------------------------------------------------------------------|
| `Error(err error)`                    | Returns `slog.Attr{}` (ignored) when `err == nil`, else `slog.Any("error", err)`     |
| `String[T ~string|~*string](k, v T)`  | `slog.Attr{}` on nil pointer or empty string, else `slog.String(k, v)`               |
| `Int[T ~int|~*int](k, v T)`           | `slog.Attr{}` on nil pointer, else `slog.Int(k, v)`                                  |
| `Int64[T ~int64|~*int64|~int32|~*int32](k, v T)` | `slog.Attr{}` on nil pointer, else `slog.Int64(k, v)`                      |

The empty-attribute sentinel is silently dropped by `slog`, so you can pass these helpers unconditionally — no `if x != nil { ... }` branches.

### Subsystem identity

| Symbol                          | Purpose                                                                                |
|---------------------------------|----------------------------------------------------------------------------------------|
| `slogx.ModuleKey` (`"subsystem"`) | Key under which subsystem names are written                                          |
| `slogx.Module("auth")`          | Single `slog.Attr` for one subsystem                                                   |
| `slogx.ModuleM("auth", "cache")` | Variadic-friendly slice of attrs for multiple subsystems                              |

`Module` pairs with the `leveled` handler: a logger built with `slog.With(slogx.Module("auth"))` is matched against the `subsystems:` block in
`config.Logger` and filtered with a single integer comparison (no per-record attribute scan).

### Context integration

| Function                                      | Purpose                                                                          |
|-----------------------------------------------|----------------------------------------------------------------------------------|
| `ContextWithLogger(ctx, *slog.Logger)`        | Store logger under a typed key; no-op if one is already present; panics on nil   |
| `FromContext(ctx)`                            | Retrieve stored logger or `nil`                                                   |
| `FromContextOrDefault(ctx)`                   | Same as above, falls back to `slog.Default()`                                     |
| `BuildLogger(ctx, base)`                      | `base.With(FieldsFromContext(ctx)...)` — used internally by interceptors          |
| `InjectLogger(ctx, base)`                     | `ContextWithLogger(ctx, BuildLogger(ctx, base))` — one call for interceptors      |
| `InjectFields(ctx, Fields)`                   | Reserve a mutable field bag in `ctx` (uses a `sync.Pool` for the wrapper)         |
| `AppendField(ctx, key, value)`                | Append one field to the bag; no-op if no bag was injected. Concurrent-safe       |
| `AppendFields(ctx, Fields)`                   | Append multiple fields; no-op if no bag was injected. Concurrent-safe            |
| `FieldsFromContext(ctx)`                      | Return a copy of the field bag (safe to mutate)                                   |
| `FieldsToAttrs(Fields)`                       | Group `Fields` by dot-separated keys into nested `slog.Group` attrs (sorted)      |
| `Fields.ToSlogArgs()` / `Fields.ToSlogAttrs()` | Adapters for `Logger.With(...)` and `Logger.LogAttrs(...)`                       |

### Dynamic level

```go
slogx.SetLevel(slog.LevelDebug)
lvl := slogx.GetLevel()
```

`slogx.GlobalLevel` is the `*slog.LevelVar` injected into every logger built by the factory. Override per-builder with
`LoggerBuilder.WithLevelVar` when you need an isolated dial.

### `ReplaceAttr` masking helper

`slogx.MaskingReplaceAttr(tags []string, mask string) func(groups []string, slog.Attr) slog.Attr` — a stdlib-compatible `ReplaceAttr` that
masks attributes whose keys are listed in `tags` (case-insensitive). Used by the factory to seed the base handler with `cfg.SensitiveTags`
before the heavier `masking` middleware is added.

### Shutdown contracts

| Interface                                          | Implemented by                                                                    |
|----------------------------------------------------|-----------------------------------------------------------------------------------|
| `HandlerWithShutdown { Shutdown(ctx) error }`      | `buffered.Handler` — drains the worker goroutine                                  |
| `InnerHandler { Inner() slog.Handler }`            | All middleware in this package (single-child)                                     |
| `InnerHandlers { Handlers() []slog.Handler }`      | `multi.Handler`                                                                   |

`slogx.Shutdown(ctx, logger)` walks the chain via these interfaces and invokes every `Shutdown` it finds, including under a `multi` fan-out.

---

## Factory (`observability/slog/factory`)

```go
import slogfactory "github.com/altessa-s/go-atlas/observability/slog/factory"

logger, err := slogfactory.New(&cfg.Logger).
    WithAppName("api-server").
    WithAppVersion("v1.4.2").
    WithServiceId(serviceID).
    WithEnableMasking().
    Build()
```

The builder accumulates errors across `With*` calls and `Build` so mask-rule parse failures don't leave you with a half-initialized logger.
After `Build` returns, `SetLevel` / `GetLevel` operate on the same `slog.LevelVar` the chain uses, giving you a runtime dial scoped to this
logger.

### Builder methods

| Method                       | Description                                                                                |
|------------------------------|--------------------------------------------------------------------------------------------|
| `New(cfg *config.Logger)`    | Construct a builder; defaults `prefixKey = "module"`, `appName` / `appVersion` from `appinfo` |
| `WithPrefixKey(key)`         | Attribute key consumed by the `prefixed` handler (default `"module"`)                       |
| `WithPrefixColors(map)`      | Per-prefix ANSI color map for the colorized handler                                         |
| `WithEnableMasking()`        | Force the advanced masking wrapper even when no rules / tags are configured                 |
| `WithAppName(s)`             | Override the `app.name` metadata attribute                                                  |
| `WithAppVersion(s)`          | Override the `app.version` metadata attribute                                               |
| `WithServiceId(s)`           | Set the `app.sid` metadata attribute                                                        |
| `WithLevelVar(*slog.LevelVar)` | Use an isolated level var instead of `slogx.GlobalLevel`                                   |
| `Build()` → `*slog.Logger`   | Assemble the chain; returns joined errors if any `With*` step recorded one                  |
| `SetLevel(slog.Level)`       | Runtime level change after `Build`                                                          |
| `GetLevel()`                 | Current level                                                                               |

### Custom output formats

`factory.RegisterHandler(formatName, factoryFn)` lets you teach the builder a new `OutputFormat` value. The registered function gets the
target writer, the `*config.Logger`, and the pre-populated `*slog.HandlerOptions` (including the seeded `ReplaceAttr`), and returns the base
handler to wrap; the factory still adds `prefixed`, `leveled`, `masking`, and `buffered` on top.

### YAML config

`config.Logger` (full reference in `config/logger.go`):

```yaml
logger:
  level: info                 # error | warning | info | debug | none
  output: stdout              # stdout | stderr
  outputFormat: text          # text | json (registered custom names also work)
  colorized: true             # only honored for text format on a TTY
  outputSource: false         # include file:line in records
  timeFormat: ""              # Go time.Format pattern (default RFC3339Nano)
  appGroupName: app           # group name for {name, version, sid} metadata
  tags:                       # attached to every record via Logger.With
    deployment: prod
  sensitiveTags:              # case-insensitive; also auto-enables the masking wrapper
    - api_key
  maskString: "****"          # default replacement when a mask doesn't override it
  enableDefaultMasks: false   # enable the curated default set even with no rules / tags
  maskRules:                  # custom masks (see masking handler below)
    - field: ssn
      type: partial
      params: { show_first: 0, show_last: 4 }
    - pattern: '(?i).*token.*'
      type: smart
  buffer:
    enabled: false
    size: 100
    bypassLevel: error        # records at or above this level write synchronously
  subsystems:                 # per-subsystem level overrides for the leveled handler
    auth: debug
    cache: warning
```

The factory validates the block via `Logger.Validate` (ozzo-validation) before doing any work.

---

## Handlers

Every middleware is independently importable, so you can compose your own chain without going through the factory. All handlers are safe for
concurrent use; `WithAttrs` / `WithGroup` return new instances rather than mutating the receiver.

### `handler/buffered`

```go
import "github.com/altessa-s/go-atlas/observability/slog/handler/buffered"

h := buffered.NewHandler(inner,
    buffered.WithBufferSize(2048),
    buffered.WithBypassLevel(slog.LevelWarn),
)
```

A background worker drains a `chan slog.Record` into `inner`. Records at or above `BypassLevel` (default `slog.LevelError`) flush the
buffer and write synchronously, so an error still reaches the writer if the process crashes before the next periodic drain. `Shutdown(ctx)`
stops the worker and drains the remaining records before returning.

| Option              | Default          | Description                                              |
|---------------------|------------------|----------------------------------------------------------|
| `WithBufferSize`    | `100`            | Channel capacity for queued records                       |
| `WithBypassLevel`   | `slog.LevelError`| Minimum level that bypasses the buffer and writes synchronously |

### `handler/colorized`

```go
import "github.com/altessa-s/go-atlas/observability/slog/handler/colorized"

h := colorized.NewHandler(os.Stdout,
    colorized.WithLevel(slog.LevelDebug),
    colorized.WithAddSource(),
    colorized.WithTimeFormat(time.RFC3339Nano),
    colorized.WithAttributeColors(map[string][]int{"module": {46}}),
    colorized.WithPrefixAttributeKey("module"),
)
```

Human-readable text format with ANSI colors. Disables color automatically with `WithNoColor`, or when the factory detects the writer is not
a TTY (`isatty`).

| Option                      | Default           | Description                                                    |
|-----------------------------|-------------------|----------------------------------------------------------------|
| `WithLevel(slog.Leveler)`   | `slog.LevelError` | Minimum level                                                  |
| `WithAddSource()`           | off               | Include source file, line, and function                        |
| `WithNoColor()`             | off               | Disable ANSI colors (forced by factory when writer is not a TTY) |
| `WithTimeFormat(string)`    | `time.RFC3339Nano`| Timestamp format                                               |
| `WithReplaceAttr(fn)`       | nil               | stdlib-compatible attribute rewriter (seeded with masking by factory) |
| `WithAttributeColors(map)`  | nil               | Per-key ANSI color overrides                                   |
| `WithPrefixAttributeKey(s)` | empty             | Attribute pulled out of the record and rendered as a leading tag |

### `handler/leveled`

```go
import "github.com/altessa-s/go-atlas/observability/slog/handler/leveled"

h := leveled.NewHandler(inner,
    leveled.WithDefaultLevel(slog.LevelInfo),
    leveled.WithSubsystemLevels(map[string]slog.Level{
        "auth":  slog.LevelDebug,
        "cache": slog.LevelWarn,
    }),
)
```

Filters records by the `subsystem` attribute. When the subsystem was captured via `WithAttrs` (i.e. the logger was created with
`slog.With(slogx.Module(...))`), `Enabled` performs a single comparison against the cached level — the zero-cost fast path. Otherwise
`Handle` scans the record attributes to find the key.

| Option                 | Default                  | Description                                              |
|------------------------|--------------------------|----------------------------------------------------------|
| `WithDefaultLevel`     | `slog.LevelInfo`         | Fallback level for subsystems not in the map             |
| `WithSubsystemLevels`  | empty                    | Map of subsystem name → level. Copied at option time.    |
| `WithSubsystemKey`     | `"subsystem"`            | Attribute key consulted to identify the subsystem        |

### `handler/masking`

```go
import "github.com/altessa-s/go-atlas/observability/slog/handler/masking"

h := masking.NewHandler(inner,
    masking.WithDefaults(),                                   // curated default set
    masking.WithField("ssn", masking.PartialMask(0, 4, "*")), // override one field
    masking.WithPattern(`(?i).*authorization`, masking.FullMask()),
    masking.WithDefaultMask(masking.FixedMask("[redacted]")),
)
```

Replaces sensitive values before they reach `inner`. Supports exact field names, regex patterns, and reflection-based descent into
`slog.KindAny` values (capped at depth 10 to stop on cyclic structs). A per-handler path memoization map caches resolved masks per field
path and is reset atomically once it reaches 4096 entries, so dynamic group names (request IDs, tenant slugs) can't grow it unboundedly.

| Option                       | Description                                                                                |
|------------------------------|--------------------------------------------------------------------------------------------|
| `WithField(name, MaskFunc)`  | Exact-match field; case-insensitive                                                         |
| `WithPattern(regex, MaskFunc)` | Regex against the field path when nested masking is on, otherwise the bare key            |
| `WithDefaults()`             | Curated set: `password`, `token`, `secret`, `api_key`, `authorization`, `credit_card`, `email`, `phone`, plus `(?i).*(password|secret|token|_key).*` patterns — also enables nested masking |
| `WithDefaultMask(MaskFunc)`  | Fallback mask for the YAML `sensitiveTags` set; default is `FullMask`                       |
| `WithMaskNestedFields(bool)` | Descend into nested groups; on by default via `WithDefaults`                                |
| `WithCaseSensitive(bool)`    | Switch matching to case-sensitive (default is case-insensitive)                             |

**Mask functions** (build your own `MaskFunc func(string) string` or use these built-ins):

| Function                       | Behavior                                                                                |
|--------------------------------|-----------------------------------------------------------------------------------------|
| `FullMask()`                   | Replace every rune with `*`                                                              |
| `FixedMask(s)`                 | Replace the entire value with `s` (typically the configured `MaskString`)               |
| `PartialMask(first, last, ch)` | Keep `first` leading and `last` trailing runes, mask the middle with `ch`               |
| `CachedPartialMask(...)`       | Same as above with pre-computed mask strings for common lengths (faster on hot paths)   |
| `SmartMask()`                  | Length-aware: short values → `Fixed`, medium → `Partial`, long → keep more visible      |
| `EmailMask()`                  | Mask local part of `user@host`, keep domain                                              |
| `PhoneMask()`                  | Mask the middle digits of a phone-like string                                            |
| `CreditCardMask()`             | Keep last four digits                                                                    |
| `URLMask()`                    | Strip user-info and mask sensitive path/query components                                 |
| `S3URLMask()`                  | Redact bucket/object paths in `s3://`, `gs://`, `https://*.s3.*.amazonaws.com` URLs     |
| `HashMask(prefix)`             | Replace value with `prefix` + short SHA-256 hash (stable, non-reversible identifier)    |
| `PatternMask(regex, MaskFunc)` | Apply `MaskFunc` only to substrings matching `regex` (e.g. credit-card-looking digits)  |

**Registry.** YAML `maskRules` resolve through a process-wide registry. Built-in masks are pre-registered under names like `"smart"`,
`"partial"`, `"full"`, `"fixed"`, `"email"`, `"phone"`, `"credit_card"`, `"url"`, `"s3url"`, `"hash"`. Plug in your own with
`masking.Register("name", fn)` (no-arg) or `masking.RegisterFactory("name", factory)` (consumes the YAML `params:` map).

### `handler/multi`

```go
import "github.com/altessa-s/go-atlas/observability/slog/handler/multi"

h := multi.NewHandler(stdoutHandler, fileHandler, otelHandler)
// or, for parallel fan-out:
h := multi.NewConcurrentHandler(stdoutHandler, fileHandler, otelHandler)
```

Fans out each record to every child handler whose `Enabled` returns true. On Go 1.26+ this delegates to `slog.MultiHandler` from the
standard library. Implements `slogx.InnerHandlers`, so `slogx.Shutdown` traverses every child automatically.

### `handler/prefixed`

```go
import "github.com/altessa-s/go-atlas/observability/slog/handler/prefixed"

h := prefixed.NewHandler(inner,
    prefixed.WithPrefix("module"),
    prefixed.WithPrefixFormatter(prefixed.DefaultFormatter),
)
```

Pulls attributes named by `WithPrefix` out of the record and renders them as a leading tag. The factory uses `DefaultFormatter` (`[api:server]`)
for text output and `JsonFormatter` (`api:server`) for JSON, so the tag flows naturally into either format.

| Option                  | Default                | Description                                                  |
|-------------------------|------------------------|--------------------------------------------------------------|
| `WithPrefix(key)`       | empty (required)       | Attribute key whose values become prefixes                    |
| `WithPrefixFormatter(fn)` | `DefaultFormatter`   | Function turning `[]slog.Value` + delimiter into one value    |
| `WithPrefixesDelimiter` | `":"`                  | Separator between multiple prefix values                      |

---

## Sequence diagram — full chain on a record

```
logger.Info("login", "user_id", 42, slogx.Module("auth"))
        │
        ▼
[buffered] ── async unless level ≥ BypassLevel ──┐
        │                                        │
        ▼                                        ▼
   (worker)                                  (sync write for Error)
        │
        ▼
[masking] ── rewrites attrs (user_id stays, "password" / "token" become "****")
        │
        ▼
[leveled] ── consults subsystem="auth"; drops record if below configured "auth" level
        │
        ▼
[prefixed] ── pulls module="auth", formats "[auth] login user_id=42"
        │
        ▼
[colorized] | JSON ── writes to os.Stdout / os.Stderr
```

The wrapping order puts `masking` above `leveled` in the stack, but `Enabled` is checked through the whole chain before `Handle` runs — so a
record that `leveled` would drop never reaches the masking pass either.

---

## What's provided

- A `slogx` core: nil-safe attribute helpers, context-stored loggers and fields, a runtime `LevelVar`, and `Shutdown` chain traversal.
- Six independently importable `slog.Handler` middlewares — `buffered`, `colorized`, `leveled`, `masking`, `multi`, `prefixed`.
- A YAML-driven factory that assembles the chain from `config.Logger`, accumulates option errors, and registers an `OnShutdown` hook for the buffer.
- A registry-driven masking layer with built-in masks for common PII shapes (email, phone, credit card, URLs, S3 paths) and a hook for custom
  factories that consume the YAML `params:` map.
