# commands

```go
import "github.com/altessa-s/go-atlas/tools/codegen/goconfig/commands"
```

Package `commands` wires the cobra root command for goconfig and registers all subcommands. `New` returns the root `cobra.Command` for
embedding in larger CLI trees. `Run` is a convenience wrapper that sets `os.Args` and executes the command in one call.

## Entry points

| Function | Description                                                                                        |
|----------|----------------------------------------------------------------------------------------------------|
| `New`    | Returns a `*cobra.Command` with the `convert` subcommand already registered                        |
| `Run`    | Creates a fresh root command, sets args, and executes; returns any error from the command tree      |

## Subpackages

| Package                | Description                                                                              |
|------------------------|------------------------------------------------------------------------------------------|
| [convert](./convert)   | Implements the `convert` subcommand with format-specific converters                      |
