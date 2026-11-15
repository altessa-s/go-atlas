# document

```go
import "github.com/altessa-s/go-atlas/store/mongo/internal/document"
```

Internal struct parsing and BSON document conversion. MongoDB client lifecycle and key management remain in the [root package](../../README.md).

## Key types

| Type | Responsibility |
|------|----------------|
| `Parser` | Cached type metadata, BSON tags and encryption schemas |
| `Engine` | Insert fields and update set/unset documents |
| `EncryptFunc` | Caller-supplied field encryption |
| `EncryptionModel` | Schema values shared with the public root aliases |

## Contract

A nil encryptor rejects non-nil fields marked for encryption. Nested updates replace whole subdocuments. Custom BSON marshalers remain scalar values. Returned parser metadata belongs to the caller; cached descriptions retain no entity instances.

Parser options are generated from `parser_options.go`. Parser types and options are internal. The root uses this package directly; encryption schema values remain part of its client configuration.
