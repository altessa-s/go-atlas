# convert

```go
import "github.com/altessa-s/go-atlas/tools/codegen/goconfig/commands/convert"
```

Package `convert` implements the `goconfig convert` subcommand and the converters that transform configuration files between YAML, TOML,
`.env`, and Markdown formats. Supports directory merging with concurrent processing above a configurable file threshold.

## Converters

| Type                        | Description                                                                     |
|-----------------------------|---------------------------------------------------------------------------------|
| `ConfigToEnvConverter`      | YAML/TOML to `.env` with optional comment tracking and YAML auto-uncomment      |
| `EnvToConfigConverter`      | `.env` to YAML/TOML with automatic type inference for values                    |
| `ConfigToConfigConverter`   | YAML to TOML or TOML to YAML direct format conversion                          |
| `ConfigToMarkdownConverter` | YAML/TOML to Markdown reference tables with optional Claude AI descriptions     |

## Naming conventions

Environment variable keys follow the go-tools config parser conventions. Nested structures are delimited by `__` (double underscore)
and `camelCase` field names become `SCREAMING_SNAKE_CASE` (e.g. `grpc.interceptors.realIp` becomes `GRPC__INTERCEPTORS__REAL_IP`).
