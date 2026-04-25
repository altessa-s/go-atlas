# rootcmd

```go
import "github.com/altessa-s/go-atlas/tools/codegen/shared/rootcmd"
```

Package `rootcmd` provides a reusable foundation for cobra-based CLI tools in the go-atlas codegen family. Handles version display
sourced from `appinfo.Version`, command-suggestion distance, and panic recovery with full stack traces written to stderr.

## Functions

| Function | Description                                                                                        |
|----------|----------------------------------------------------------------------------------------------------|
| `New`    | Returns a configured `*cobra.Command` from a `Config`; callers extend or execute it directly       |
| `Run`    | Convenience wrapper that sets args, installs panic recovery, and calls `Execute`                    |

## Config fields

| Field         | Description                                                                                   |
|---------------|-----------------------------------------------------------------------------------------------|
| `Use`         | Command name (e.g. `"optgen"`, `"goconfig"`)                                                  |
| `Short`       | One-line description shown in help output                                                     |
| `Long`        | Multi-line description shown in detailed help                                                 |
| `Example`     | Usage examples shown in help output                                                           |
| `Subcommands` | Slice of `*cobra.Command` to register as subcommands                                          |
| `Out`         | Custom `io.Writer` for stdout (useful for tests; defaults to `os.Stdout`)                     |
| `Err`         | Custom `io.Writer` for stderr (useful for tests; defaults to `os.Stderr`)                     |
