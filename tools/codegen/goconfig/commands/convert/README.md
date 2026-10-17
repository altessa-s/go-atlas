# convert

```go
import "github.com/altessa-s/go-atlas/tools/codegen/goconfig/commands/convert"
```

Package `convert` implements the `goconfig convert` subcommand and the converters that transform configuration files between YAML, TOML, `.env`, and
Markdown formats. Supports directory merging; five or more files matching the selected source format are loaded concurrently.

## Converters

| Type                        | Description                                                                     |
|-----------------------------|---------------------------------------------------------------------------------|
| `ConfigToEnvConverter`      | YAML/TOML to `.env` with optional comment tracking and YAML auto-uncomment      |
| `EnvToConfigConverter`      | `.env` to YAML/TOML with automatic type inference for values                    |
| `ConfigToConfigConverter`   | YAML to TOML or TOML to YAML direct format conversion                          |
| `ConfigToMarkdownConverter` | YAML/TOML to Markdown reference tables with optional Claude AI descriptions     |

`New` returns the `convert` `*cobra.Command`; `ClaudeClient` (`NewClaudeClient(apiKey)`) generates the short field descriptions used by
`--use-claude` and is not safe for concurrent use.

## Naming conventions

Environment variable keys follow the [config/loader](../../../../../config/loader) conventions. Nested structures are delimited by `__` (double
underscore) and `camelCase` field names become `SCREAMING_SNAKE_CASE` (e.g. `grpc.interceptors.realIP` becomes `GRPC__INTERCEPTORS__REAL_IP`).
