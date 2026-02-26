# lockbox

```go
import "github.com/altessa-s/go-atlas/security/secrets/providers/lockbox"
```

Package `lockbox` provides a Yandex Cloud Lockbox provider for the Atlas secrets system. Features dual-client
architecture with IAM authentication, CRUD operations, and label-based filtering.

## Usage

```go
store, err := lockbox.New[MySecret](
    folderID, keyID, serviceKeyID, privateKey,
    lockbox.WithKeyDecoder(base64Dec),
    lockbox.WithValueDecoder(jsonDec),
)
```
