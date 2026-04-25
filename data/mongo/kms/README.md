# kms

```go
import "github.com/altessa-s/go-atlas/data/mongo/kms"
```

Package `kms` defines the `Provider` interface for Key Management Service integration with MongoDB Client-Side Field Level Encryption.

## Subpackages

| Package                    | Description                        |
|----------------------------|------------------------------------|
| [local](./local)           | Local key management               |
| [aws](./aws)               | AWS KMS provider                   |
| [azure](./azure)           | Azure Key Vault provider           |
| [gcp](./gcp)               | Google Cloud KMS provider          |
| [factory](./factory)       | Configuration-based KMS creation   |
