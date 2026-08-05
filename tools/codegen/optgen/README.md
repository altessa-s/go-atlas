# optgen

Code generator for the functional option pattern. Scans Go structs for fields annotated with
`opt`/`optgen`/`optval`/`optcheck` tags and generates type-safe `WithXxx` option functions, default
constructors, and validation code with an extensible plugin system.

## Tag format

| Tag        | Purpose                                                                          |
|------------|----------------------------------------------------------------------------------|
| `opt`      | Option name for the `WithXxx` function; use `-` to skip generation. Omit it to derive the name from the field (see [Option naming](#option-naming)) |
| `optgen`   | Generator directives: `default=...`, `append`, `notnil`, `manual`, `boolFlag`    |
| `optval`   | Value modifiers applied in order: `trimspaces`, `lower`, `upper`, `dedup`        |
| `optcheck` | Validation rules: `required`, `nonzero`, `minlen=N`, `maxlen=N`, `oneof=[a,b,c]`|

## Option naming

Without an `opt` tag the option name is derived from the field name, with every word that is a known initialism upper-cased — Go style requires an
initialism to keep a consistent case throughout an identifier:

| Field           | Generated                |
|-----------------|--------------------------|
| `ttl`           | `WithTTL`                |
| `httpClient`    | `WithHTTPClient`         |
| `entityId`      | `WithEntityIDExtractor`* |
| `maxIdleTime`   | `WithMaxIdleTime`        |
| `grpcOptions`   | `WithGrpcOptions`        |

<sub>* for a field named `entityIdExtractor`.</sub>

The table is golint's `commonInitialisms` plus `DB`. `gRPC` is deliberately absent: the repo spells it `Grpc` throughout (`config.Grpc`,
`DefaultGrpc`), and a lone `WithGRPCOptions` would be less consistent, not more. To override a spelling the table gets wrong, name the option
explicitly — the `opt` tag always wins:

```go
type options struct {
    ttl time.Duration                // -> WithTTL
    uid string        `opt:"Uid"`    // -> WithUid
}
```

## CLI commands

| Command        | Description                                                               |
|----------------|---------------------------------------------------------------------------|
| `generate`     | Parse the struct and generate option functions into `*_gen.go`            |
| `list-plugins` | List all registered plugins (built-in and external)                       |
| `version`      | Print version information                                                 |

## Configuration

Settings are loaded from `.optgen.yaml` (searched from the working directory up to the project root).
CLI flags always take precedence over config-file values. Per-package overrides are supported via the
`packages` map keyed by relative path.

## Subpackages

| Package                  | Description                                                           |
|--------------------------|-----------------------------------------------------------------------|
| [commands](./commands)   | Cobra-based CLI with `generate`, `list-plugins`, and `version`        |
| [config](./config)       | Loads and merges `.optgen.yaml` configuration with per-package overrides|
| [model](./model)         | Public data structures (`OptField`, `GenericInfo`) shared across parser|
| `internal/naming`        | Derives option names from field names, with initialism handling       |
| [plugin](./plugin)       | Public extension API: `FieldPlugin`, `TransformPlugin`, `Registry`    |
