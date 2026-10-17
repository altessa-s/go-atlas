# optgen

```go
import "github.com/altessa-s/go-atlas/tools/codegen/optgen/commands"
```

Generate functional option functions from struct field tags.

## Overview

`optgen` generates type-safe functional option functions from struct field tags. It supports defaults, validation, value transformation, and custom
plugins.

## Installation

```bash
go install github.com/altessa-s/go-atlas/cmd/optgen@latest
```

## Usage

Add a go:generate directive to your options file:

```go
//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

type options struct {
    logger     *slog.Logger  `opt:"Logger"`
    timeout    time.Duration `opt:"Timeout" optgen:"default=30*time.Second"`
    labels     []string      `opt:"Labels" optval:"trimspaces,lower,dedup"`
    custom     string        `opt:"-"` // skipped - implement manually
}
```

Run generation:

```bash
go generate ./...
```

## Tag Reference

| Tag | Purpose | Example |
|-----|---------|---------|
| `opt:"Name"` | Generate WithName option | `opt:"Logger"` |
| `opt:"-"` | Skip field generation | `opt:"-"` |
| `optgen:"default=..."` | Default value | `optgen:"default=30*time.Second"` |
| `optgen:"append"` | Slice append semantics | `optgen:"append"` |
| `optgen:"notnil"` | Nil-check guard | `optgen:"notnil"` |
| `optgen:"manual"` | Skip auto-generation | `optgen:"manual"` |
| `optgen:"parseip"` | Accept IP strings, parse into `[]net.IP` | `optgen:"parseip"` |
| `optval:"..."` | Value modifiers | `optval:"trimspaces,lower"` |
| `optcheck:"..."` | Validation checks | `optcheck:"required,minlen=3"` |

## Commands

### generate

Generate functional option functions:

```bash
optgen generate [flags]

Flags:
  -t, --type string          Struct type name (default "options")
  -o, --output string        Output file (default "options_gen.go")
  -d, --directory string     Directory to scan (default ".")
      --option-type string   Name of the Option type (default "Option")
      --option-error         Generate Option as func(*T) error
      --skip-option-type     Do not emit the Option type definition
      --skip-default-func    Do not emit the defaultOptions function
      --skip-new-func        Do not emit the newOptions function
      --default-func-name    Override the default<TypeName> function name
      --new-func-name        Override the new<TypeName> function name
      --option-prefix        Prefix for generated option names (WithPrefixXxx)
      --all-fields           Include untagged fields; still honor opt names and opt:"-"
      --disable-plugins      Comma-separated plugin names to disable
      --formatter string     Formatter command (default "gofmt -w")
      --no-format            Disable formatting
      --dry-run              Preview output without writing
  -v, --verbose              Enable verbose output
      --no-config            Ignore .optgen.yaml
      --plugin stringArray   Load an external plugin (.so; darwin/linux only)
```

### list-plugins

List available plugins and modifiers:

```bash
optgen list-plugins
```

## Configuration

Create `.optgen.yaml` in your project root:

```yaml
defaults:
  all-fields: true           # Process all struct fields
  skip-option-type: false    # Skip Option type generation
  option-error: false        # Make Option return error
  formatter: "gofmt -w"      # Code formatter

packages:
  "data/cache":              # exact relative path, no globs
    all-fields: false        # Package-specific override
```

## Value Modifiers

| Modifier | Description | Types |
|----------|-------------|-------|
| `trimspaces` | Trim whitespace | string |
| `lower` | Convert to lowercase | string |
| `upper` | Convert to uppercase | string |
| `dedup` | Remove duplicates; enabled by default, disable with `nodup` | Slices of recognized comparable types, including primitives, pointers, time.Time, and time.Duration |
| `positive` | Ensure positive value | numeric |
| `nonempty` | Skip assignment for an empty string; remove empty elements from string slices | string, []string |
| `trimprefix=P` / `trimsuffix=S` | Trim a prefix / suffix | string |

`trimspaces` is applied to every string field by default; add `notrim` to `optval` to disable it.

## Validation Checks

| Check | Description | Example |
|-------|-------------|---------|
| `required` | Require a non-nil pointer or a non-empty string, slice, or map | `optcheck:"required"` |
| `minlen=N` | Minimum length | `optcheck:"minlen=3"` |
| `maxlen=N` | Maximum length | `optcheck:"maxlen=100"` |
| `oneof=[...]` | Enum validation | `optcheck:"oneof=[debug,info,warn]"` |
| `nonzero` | Non-zero value | `optcheck:"nonzero"` |

## Examples

### Basic Options

```go
type options struct {
    timeout time.Duration `opt:"Timeout" optgen:"default=30*time.Second"`
    retries int          `opt:"Retries" optgen:"default=3"`
    debug   bool         `opt:"Debug"`
}
```

### Slice Options

```go
type options struct {
    hosts   []string `opt:"Hosts" optgen:"append"`
    labels  []string `opt:"Labels" optval:"dedup,nonempty"`
}
```

### Validated Options

```go
type options struct {
    level    string `opt:"Level" optcheck:"required,oneof=[debug,info,warn,error]"`
    workers  int    `opt:"Workers" optcheck:"nonzero" optval:"positive"`
    endpoint string `opt:"Endpoint" optcheck:"required,minlen=5"`
}
```

## External Plugins

Load external plugins (.so files):

```bash
optgen generate --plugin ./myplugin.so
```

## See Also

- [tools/codegen/optgen](../../tools/codegen/optgen/) - Internal implementation
- [AGENTS.md](../../AGENTS.md#options-pattern) - Options pattern conventions