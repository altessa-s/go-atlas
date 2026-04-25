# commands

```go
import "github.com/altessa-s/go-atlas/tools/codegen/optgen/commands"
```

Package `commands` implements the cobra-based CLI for the optgen code generator. Provides three entry points: `New` returns the full
command tree, `Run` is a convenience wrapper for `main()`, and individual subcommands are available via `NewGenerate`/`NewListPlugins`.

## Entry points

| Function         | Description                                                                                 |
|------------------|---------------------------------------------------------------------------------------------|
| `New`            | Returns a `*cobra.Command` tree with `generate`, `list-plugins`, and `version` subcommands  |
| `Run`            | Sets `os.Args`, executes the root command, and returns any error                            |
| `NewGenerate`    | Returns the `generate` subcommand for embedding in custom CLI trees                         |
| `NewListPlugins` | Returns the `list-plugins` subcommand for embedding in custom CLI trees                     |

## Configuration

Configuration is loaded from `.optgen.yaml` (see `config.Config`) unless `--no-config` is set. CLI flags always take precedence over
config-file values. External plugins (`.so` files) can be loaded via `--plugin` on darwin and linux platforms.
