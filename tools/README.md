# tools

Development tools and code generators for the Atlas framework. Dev-tool dependencies (`golangci-lint`, `gosec`, `govulncheck`, `gci`, `dupl`,
`benchstat`, `go-copyright-checker`) are pinned in the separate [devtools](../devtools) module (`devtools/tools.go`, `//go:build tools`), which keeps
them out of the main module's dependency graph; the `tools.go` here only points there.

## Subpackages

| Package              | Description                                                                                    |
|----------------------|------------------------------------------------------------------------------------------------|
| [codegen](./codegen) | Code generation tools: `optgen` (functional options) and `goconfig` (config file conversion)   |
