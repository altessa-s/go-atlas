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
| `StrictDecoder` | `DecodeStrict(reader, any)`        | Optional: decodes and rejects keys that bind to no struct field |
| `KeyNode`      | `IsNull`, `Fields`, `Entries`, `Elems` | A document value bound to destination types on demand |

`Preprocessor` is optional. Backends that support directives like `!include` implement it to transform file
content before decoding.

`StrictDecoder` is optional. The loader decodes with it unless `loader.WithAllowUnknownFields()` is set; its error wraps
`backend.ErrUnknownField` and names every key that binds to no field of a destination struct. Map entry keys are never unknown, though
struct values in a map are checked; content under interfaces and types with their own unmarshaler is free-form. Without it the loader decodes with `Decode` and cannot detect unknown keys.

`KeyDecoder` is optional too. The loader uses it to tell an explicit `false`, `0` or `""` from an omitted key, so `default` tags
apply only to the latter. A `KeyNode` binds struct fields, map keys and sequence elements exactly as the backend's `Decode` does;
both built-in backends implement it by decoding into shadow types typed after the destination. Without it the loader falls back to
a generic decode matched by tag or field name.

## Migrating a `KeyDecoder` (breaking change)

`KeyDecoder.DecodeKeys` used to return `map[string]any` keyed by source spelling. It now returns a `KeyNode`, so map keys, merges and
embedding are bound against the destination type exactly as the backend decodes them. A backend that still implements the old
signature no longer satisfies `KeyDecoder`; without a compile-time assertion the loader silently falls back to the generic decode.
Implement the new methods, and assert the interface so the compiler catches a mismatch:

```go
func (b *Backend) DecodeKeys(r io.Reader) (backend.KeyNode, error) {
	// Parse r and return its root value; an empty document is a null value.
}

var _ backend.KeyDecoder = (*Backend)(nil)
```

A `KeyNode` returns, per call, the children it binds to the given type: `Fields` keyed by field index (an embedded struct the backend
flattens maps to a node holding its fields), `Entries` keyed by the decoded map key, `Elems` in decoded order. Both built-in
`KeyNode` implementations are safe for concurrent use.

## Built-in backends

| Package  | Format | Extensions      | Tag    |
|----------|--------|-----------------|--------|
| `yaml3`  | YAML   | `.yaml`, `.yml` | `yaml` |
| `toml`   | TOML   | `.toml`, `.tml` | `toml` |
