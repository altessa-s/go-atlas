# optgen

Code generator for the functional option pattern. Scans Go structs for fields annotated with `opt`/`optgen`/`optval`/`optcheck` tags and generates
type-safe `WithXxx` option functions, default constructors, and validation code with an extensible plugin system.

## Tag format

| Tag        | Purpose                                                                          |
|------------|----------------------------------------------------------------------------------|
| `opt`      | Option name for the `WithXxx` function; use `-` to skip generation. Omit it to derive the name from the field (see [Option naming](#option-naming)) |
| `optgen`   | Generator directives: `default=...`, `append`, `notnil`, `manual`, `parseip`, `err=...` |
| `optval`   | Value modifiers: `trimspaces`, `lower`, `upper`, `trimprefix=...`, `trimsuffix=...`, `dedup`, `nonempty`, `positive`; `invert` / `param` for bool fields. `trimspaces` applies to string fields by default; `notrim` disables it |
| `optcheck` | Validation rules: `required`, `nonzero`, `minlen=N`, `maxlen=N`, `oneof=[a,b,c]`|

## Option naming

For fields selected by `--all-fields`, `all-fields: true`, or an `optgen`/`optval`/`optcheck` tag, an omitted `opt` tag derives the option name from the
field name, with every word that is a known initialism upper-cased — Go style requires an initialism to keep a consistent case throughout an identifier:

| Field           | Generated                |
|-----------------|--------------------------|
| `ttl`           | `WithTTL`                |
| `httpClient`    | `WithHTTPClient`         |
| `entityId`      | `WithEntityIDExtractor`* |
| `maxIdleTime`   | `WithMaxIdleTime`        |
| `grpcOptions`   | `WithGrpcOptions`        |

<sub>* for a field named `entityIdExtractor`.</sub>

The table is golint's `commonInitialisms` plus `DB`. `gRPC` is deliberately absent: the repo spells it `Grpc` throughout (`grpcconfig.Config`,
`DefaultGrpc`), and a lone `WithGRPCOptions` would be less consistent, not more. To override a spelling the table gets wrong, name the option explicitly
— the `opt` tag always wins:

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
| `list-plugins` | List registered field plugins, type-default providers, modifier plugins, and validation keys; standalone transform plugins are not listed                       |

The root command prints version information with `--version`.

## Configuration

Settings are loaded from `.optgen.yaml`, searched from `--directory` (default `.`) up to the project root. Explicit CLI flags override corresponding
config values, except `--plugin`, which adds paths after configured plugins. Per-package overrides use the `packages` map keyed by relative path;
package-specific `disable-plugins` is currently ignored by the CLI.

## Subpackages

| Package                  | Description                                                           |
|--------------------------|-----------------------------------------------------------------------|
| [commands](./commands)   | Cobra-based CLI with `generate` and `list-plugins`                    |
| [config](./config)       | Loads and merges `.optgen.yaml` configuration with per-package overrides|
| [model](./model)         | Public data structures (`OptField`, `GenericInfo`) shared across parser|
| `internal/naming`        | Derives option names from field names, with initialism handling       |
| [plugin](./plugin)       | Public extension API: `FieldPlugin`, `TransformPlugin`, `Registry`    |
