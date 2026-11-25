# kms

```go
import "github.com/altessa-s/go-atlas/store/mongo/kms"
```

Package `kms` defines the `MasterKeyProvider` and optional `TLSConfigurer` interfaces for MongoDB Client-Side Field Level Encryption.

## Subpackages

| Package                                     | Description                      |
|---------------------------------------------|----------------------------------|
| [providers/local](./providers/local)        | Local key management             |
| [providers/aws](./providers/aws)            | AWS KMS provider                 |
| [providers/azure](./providers/azure)        | Azure Key Vault provider         |
| [providers/gcp](./providers/gcp)            | Google Cloud KMS provider        |
| [factory](../../../factory/store/mongo/kms) | Configuration-based KMS creation |
