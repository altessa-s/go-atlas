# codegen

Code generation tools for the Atlas framework. Each tool is a standalone `main` package invoked via `go run` or `go generate` directives
and shares the common cobra-based CLI foundation from the [shared/rootcmd](./shared/rootcmd) package.

## Tools

| Tool                       | Description                                                                              |
|----------------------------|------------------------------------------------------------------------------------------|
| [optgen](./optgen)         | Generates type-safe `WithXxx` functional option functions from annotated struct fields    |
| [goconfig](./goconfig)     | Converts configuration files between YAML, TOML, `.env`, and Markdown formats            |

## Subpackages

| Package                    | Description                                                                              |
|----------------------------|------------------------------------------------------------------------------------------|
| [shared/rootcmd](./shared/rootcmd) | Reusable cobra root command with version display, suggestion distance, and panic recovery |
