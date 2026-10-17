# config

```go
import "github.com/altessa-s/go-atlas/tools/codegen/optgen/config"
```

Package `config` loads and merges `.optgen.yaml` configuration files for the optgen code generator. `FindConfigFile` walks from a start directory toward
the filesystem root, stopping at the first `.optgen.yaml` or at a project boundary (`go.mod` / `.git`).

## Functions

| Function           | Description                                                                               |
|--------------------|-------------------------------------------------------------------------------------------|
| `Load`             | Parse an `.optgen.yaml` file at the given path into a `Config`                            |
| `FindConfigFile`   | Search parent directories for `.optgen.yaml`, return config path and project root          |

## Key types

| Type             | Description                                                                                 |
|------------------|---------------------------------------------------------------------------------------------|
| `Config`         | Top-level configuration with `Defaults` and per-package `Packages` map                      |
| `Defaults`       | Global defaults: type name, output file, option type, formatter, plugin lists               |
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
| `AllFields`       | Process every struct field, not only tagged ones (`all-fields`)                            |
| `SkipOptionType`  | Skip generating the `Option` type definition                                               |
| `SkipDefaultFunc` | Skip generating the `defaultOptions` function                                              |
| `SkipNewFunc`     | Skip generating the `newOptions` function                                                  |
| `DefaultFuncName` | Custom name for the default function                                                       |
| `NewFuncName`     | Custom name for the new function                                                           |
| `OptionPrefix`    | Prefix for generated option function names                                                 |
| `Plugins`         | External plugin (`.so`) paths to load                                                      |
| `Verbose`         | Enable verbose output                                                                      |
| `NoFormat`        | Disable formatting of generated files                                                      |

YAML keys are kebab-case (`option-type`, `all-fields`, `disable-plugins`). `Config.GetPackageConfig(pkgPath)` returns the `Defaults` merged with the
`packages` override for that path.
