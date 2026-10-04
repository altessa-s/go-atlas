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
| `KeyDecoder`   | `DecodeKeys(reader)`                 | Optional: reports which values a document sets |
| `KeyNode`      | `IsNull`, `Fields`, `Entries`, `Elems` | A document value bound to destination types on demand |

`Preprocessor` is optional. Backends that support directives like `!include` implement it to transform file
content before decoding.

`KeyDecoder` is optional too. The loader uses it to tell an explicit `false`, `0` or `""` from an omitted key, so `default` tags
apply only to the latter. A `KeyNode` binds struct fields, map keys and sequence elements exactly as the backend's `Decode` does;
both built-in backends implement it by decoding into shadow types typed after the destination. Without it the loader falls back to
a generic decode matched by tag or field name.

## Built-in backends

| Package  | Format | Extensions      | Tag    |
|----------|--------|-----------------|--------|
| `yaml3`  | YAML   | `.yaml`, `.yml` | `yaml` |
| `toml`   | TOML   | `.toml`, `.tml` | `toml` |
