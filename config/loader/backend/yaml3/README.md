# yaml3

```go
import "github.com/altessa-s/go-atlas/config/loader/backend/yaml3"
```

Package `yaml3` provides a YAML backend for the configuration loader. It uses
[gopkg.in/yaml.v3](https://github.com/go-yaml/yaml) for parsing and supports `!include` directives for
splitting configuration across multiple files.

## Details

| Property        | Value              |
|-----------------|--------------------|
| Struct tag      | `yaml`             |
| File extensions | `.yaml`, `.yml`    |
| Preprocessor    | Yes (`!include`)   |

The zero value of `Backend` is ready to use. This is the default backend when `nil` is passed to
`loader.New`.

## !include directive

Split large configs into smaller files using `!include <filename>` in any YAML value position.

Security constraints:
- Only `.yaml` and `.yml` extensions are allowed
- Included paths must stay within the config root directory (no path traversal)
- Circular includes are detected and rejected
- Maximum nesting depth is 10 levels
