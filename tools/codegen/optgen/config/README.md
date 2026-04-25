# config

```go
import "github.com/altessa-s/go-atlas/tools/codegen/optgen/config"
```

Package `config` loads and merges `.optgen.yaml` configuration files for the optgen code generator. `FindConfigFile` walks from a start
directory toward the filesystem root, stopping at the first `.optgen.yaml` or at a project boundary (`go.mod` / `.git`).

## Functions

| Function           | Description                                                                               |
|--------------------|-------------------------------------------------------------------------------------------|
| `Load`             | Parse an `.optgen.yaml` file at the given path into a `Config`                            |
| `FindConfigFile`   | Search parent directories for `.optgen.yaml`, return config path and project root          |

## Key types

| Type             | Description                                                                                 |
|------------------|---------------------------------------------------------------------------------------------|
| `Config`         | Top-level configuration with `Defaults` and per-package `Packages` map                      |
| `Defaults`       | Global defaults: type name, output file, option type, formatter, plugin list                |
| `PackageConfig`  | Per-package overrides keyed by relative path; unset fields inherit from `Defaults`           |

## Defaults fields

| Field             | Description                                                                                |
|-------------------|--------------------------------------------------------------------------------------------|
| `Type`            | Struct type name to scan for option fields                                                 |
| `Output`          | Output file name for generated code                                                        |
| `OptionType`      | Name of the generated `Option` type                                                        |
| `OptionError`     | Generate `Option` as `func(*T) error` instead of `func(*T)`                                |
| `Formatter`       | Command to format generated files (default: `gofmt -w`)                                    |
| `DisablePlugins`  | List of plugin names to disable                                                            |
