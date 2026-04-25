# tools

Development tools and code generators for the Atlas framework. The root `tools.go` file pins tool dependencies (`golangci-lint`, `gosec`,
`govulncheck`, `gci`, `dupl`) under a `//go:build tools` constraint so they are tracked by `go.mod` without being compiled into binaries.

## Subpackages

| Package              | Description                                                                                    |
|----------------------|------------------------------------------------------------------------------------------------|
| [codegen](./codegen) | Code generation tools: `optgen` (functional options) and `goconfig` (config file conversion)   |
