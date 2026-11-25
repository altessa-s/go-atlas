# document

```go
import "github.com/altessa-s/go-atlas/store/mongo/internal/document"
```

Package `document` implements the [store](../../README.md#write-documents) field-encryption policy. The
[MongoDB translator](../../../../mapping/behavior/translators/mongo/README.md) owns the BSON layout and update operators.

## Key types

| Type | Responsibility |
|------|----------------|
| `Fields` | Resolves an encryption declaration by its Go field and transforms selected values into ciphertext |
| `EncryptionModel`, `EncryptionField` | Per-type rules configured through the store's public aliases |
| `EncryptFunc` | Applies the selected algorithm and key to a registry-encoded BSON value |

## Field selection

Register each model for the type that declares its exported fields. A model rule takes precedence over the field's encryption tag. The default tag
key is `encryption`, and a tag such as `encryption:"deterministic,user_email"` identifies the algorithm and key alternative name.

The builder supplies the Go type and field path, stored BSON name, and a lazy encoder bound to the effective registry. An encryption or encoding
failure aborts the conversion. An unsupported placement, such as a codec-owned parent containing a separately encrypted field, also fails the conversion.
