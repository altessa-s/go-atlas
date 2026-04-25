# backend

```go
import "github.com/altessa-s/go-atlas/config/loader/backend"
```

Package `backend` defines the `Backend` interface for configuration file format parsers. Implement this interface
to add support for new configuration formats (JSON, INI, etc.).

## Interfaces

| Interface      | Methods                              | Description                              |
|----------------|--------------------------------------|------------------------------------------|
| `Backend`      | `Decode`, `FileExtensions`, `StructTagName` | Main backend contract               |
| `Decoder`      | `Decode(reader, any)`                | Decodes a reader into a struct           |
| `Preprocessor` | `Preprocess(content, currentDir, rootDir)` | Optional content transformation    |

`Preprocessor` is optional. Backends that support directives like `!include` implement it to transform file
content before decoding.

## Built-in backends

| Package  | Format | Extensions      | Tag    |
|----------|--------|-----------------|--------|
| `yaml3`  | YAML   | `.yaml`, `.yml` | `yaml` |
| `toml`   | TOML   | `.toml`, `.tml` | `toml` |
